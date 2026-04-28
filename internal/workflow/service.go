package workflow

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	jobscmd "github.com/zaneway/AutoCertX/internal/application/command/jobs"
	certasset "github.com/zaneway/AutoCertX/internal/domain/certificateasset"
	certrequest "github.com/zaneway/AutoCertX/internal/domain/certificaterequest"
	"github.com/zaneway/AutoCertX/internal/domain/domains"
	"github.com/zaneway/AutoCertX/internal/domain/issuer"
	issueflow "github.com/zaneway/AutoCertX/internal/domain/issueworkflow"
	"github.com/zaneway/AutoCertX/internal/domain/job"
	"github.com/zaneway/AutoCertX/internal/domain/resource"
	acmedriver "github.com/zaneway/AutoCertX/internal/driver/acme"
	dnsdriver "github.com/zaneway/AutoCertX/internal/driver/dns"
)

const (
	jobTypeStartIssueWorkflow    = "start_issue_workflow"
	jobTypeContinueIssueWorkflow = "continue_issue_workflow"
)

// SubmitInput is the public write model for request submission.
type SubmitInput struct {
	RequestType         string
	DomainIDs           []string
	CAAccountID         string
	AssetID             string
	CertificateType     string
	ChallengeType       string
	IdempotencyKey      string
	DeploymentTargetIDs []string
}

// AcceptedResult reports the accepted request and scheduler job ids.
type AcceptedResult struct {
	RequestID string
	JobID     string
}

// ProcessResult controls how the scheduler should finish one issuance job.
type ProcessResult struct {
	Retryable    bool
	RetryDelay   time.Duration
	ErrorCode    string
	ErrorMessage string
}

// HTTP01Presenter is the temporary Phase A abstraction for HTTP-01 presentation.
type HTTP01Presenter interface {
	Present(context.Context, HTTP01Material) error
	Cleanup(context.Context, HTTP01Material) error
}

// HTTP01Material carries the file material for one HTTP-01 challenge.
type HTTP01Material struct {
	Identifier       string
	Token            string
	KeyAuthorization string
	Path             string
}

// FakeHTTP01Presenter provides a deterministic Phase A presenter.
type FakeHTTP01Presenter struct {
	PresentErr error
	CleanupErr error
}

// Present records a successful presentation unless configured otherwise.
func (p FakeHTTP01Presenter) Present(context.Context, HTTP01Material) error {
	return p.PresentErr
}

// Cleanup records a successful cleanup unless configured otherwise.
func (p FakeHTTP01Presenter) Cleanup(context.Context, HTTP01Material) error {
	return p.CleanupErr
}

type startWorkflowPayload struct {
	RequestID string `json:"request_id"`
}

type continueWorkflowPayload struct {
	RequestID      string `json:"request_id"`
	WorkflowID     string `json:"workflow_id"`
	ExpectedStatus string `json:"expected_status"`
	Trigger        string `json:"trigger"`
}

// Service orchestrates the Phase A issuance workflow baseline.
type Service struct {
	Requests  *certrequest.Service
	Workflows *issueflow.Service
	Assets    *certasset.Service
	Domains   *domains.Service
	Issuers   *issuer.Service
	Jobs      *jobscmd.Service
	ACME      acmedriver.Client
	DNS       dnsdriver.Executor
	HTTP01    HTTP01Presenter
	now       func() time.Time
}

// NewService constructs a workflow orchestrator.
func NewService(
	requests *certrequest.Service,
	workflows *issueflow.Service,
	assets *certasset.Service,
	domainService *domains.Service,
	issuerService *issuer.Service,
	jobs *jobscmd.Service,
	acmeClient acmedriver.Client,
	dnsExecutor dnsdriver.Executor,
	http01 HTTP01Presenter,
) (*Service, error) {
	switch {
	case requests == nil:
		return nil, fmt.Errorf("certificate request service required")
	case workflows == nil:
		return nil, fmt.Errorf("issue workflow service required")
	case assets == nil:
		return nil, fmt.Errorf("certificate asset service required")
	case domainService == nil:
		return nil, fmt.Errorf("domain service required")
	case issuerService == nil:
		return nil, fmt.Errorf("issuer service required")
	case jobs == nil:
		return nil, fmt.Errorf("jobs service required")
	case acmeClient == nil:
		return nil, fmt.Errorf("acme client required")
	case dnsExecutor == nil:
		return nil, fmt.Errorf("dns executor required")
	case http01 == nil:
		return nil, fmt.Errorf("http01 presenter required")
	default:
		return &Service{
			Requests:  requests,
			Workflows: workflows,
			Assets:    assets,
			Domains:   domainService,
			Issuers:   issuerService,
			Jobs:      jobs,
			ACME:      acmeClient,
			DNS:       dnsExecutor,
			HTTP01:    http01,
			now: func() time.Time {
				return time.Now().UTC()
			},
		}, nil
	}
}

// SubmitRequest validates and stores a request, then schedules the workflow bootstrap job.
func (s *Service) SubmitRequest(ctx context.Context, scope resource.Scope, actorID string, input SubmitInput) (AcceptedResult, error) {
	if err := scope.Validate(); err != nil {
		return AcceptedResult{}, err
	}

	requestInput, _, err := s.prepareRequestInput(scope, actorID, input)
	if err != nil {
		return AcceptedResult{}, err
	}

	request, err := s.Requests.Create(scope, requestInput)
	if errors.Is(err, resource.ErrConflict) {
		existing, lookupErr := s.Requests.GetByIdempotency(scope, input.IdempotencyKey)
		if lookupErr != nil {
			return AcceptedResult{}, lookupErr
		}
		startJob, jobErr := s.ensureStartJob(ctx, existing)
		if jobErr != nil {
			return AcceptedResult{}, jobErr
		}
		return AcceptedResult{RequestID: existing.ID, JobID: startJob.ID}, nil
	}
	if err != nil {
		return AcceptedResult{}, err
	}

	startJob, err := s.ensureStartJob(ctx, request)
	if err != nil {
		return AcceptedResult{}, err
	}
	if _, err := s.ensureRequestAccepted(scope, request); err != nil {
		return AcceptedResult{}, err
	}
	return AcceptedResult{RequestID: request.ID, JobID: startJob.ID}, nil
}

// RenewAsset creates a renewal request by reusing the asset's baseline issuance settings.
func (s *Service) RenewAsset(ctx context.Context, scope resource.Scope, actorID string, assetID string) (AcceptedResult, error) {
	asset, err := s.Assets.Get(scope, strings.TrimSpace(assetID))
	if err != nil {
		return AcceptedResult{}, err
	}

	idempotencyKey := fmt.Sprintf("renew:%s:%s", asset.ID, s.now().Format(time.RFC3339Nano))
	return s.SubmitRequest(ctx, scope, actorID, SubmitInput{
		RequestType:     certrequest.RequestTypeRenew,
		DomainIDs:       append([]string(nil), asset.DomainIDs...),
		CAAccountID:     asset.CAAccountID,
		AssetID:         asset.ID,
		CertificateType: asset.CertificateType,
		ChallengeType:   asset.ChallengeType,
		IdempotencyKey:  idempotencyKey,
	})
}

// ProcessJob advances one T08 issuance job according to the current durable workflow state.
func (s *Service) ProcessJob(ctx context.Context, record job.Job) ProcessResult {
	switch record.JobType {
	case jobTypeStartIssueWorkflow:
		return s.processStartJob(ctx, record)
	case jobTypeContinueIssueWorkflow:
		return s.processContinueJob(ctx, record)
	default:
		return ProcessResult{
			ErrorCode:    "UNSUPPORTED_JOB_TYPE",
			ErrorMessage: fmt.Sprintf("unsupported workflow job type %q", record.JobType),
		}
	}
}

func (s *Service) prepareRequestInput(scope resource.Scope, actorID string, input SubmitInput) (certrequest.CreateInput, []domains.Asset, error) {
	if strings.TrimSpace(input.CAAccountID) == "" {
		return certrequest.CreateInput{}, nil, fmt.Errorf("ca_account_id required: %w", resource.ErrValidation)
	}
	account, err := s.Issuers.Get(scope, input.CAAccountID)
	if err != nil {
		return certrequest.CreateInput{}, nil, err
	}
	if account.Status != issuer.StatusActive {
		return certrequest.CreateInput{}, nil, fmt.Errorf("ca account %s unavailable: %w", account.ID, resource.ErrUnavailable)
	}

	if len(input.DomainIDs) == 0 {
		return certrequest.CreateInput{}, nil, fmt.Errorf("domain_ids required: %w", resource.ErrValidation)
	}

	resolved := make([]domains.Asset, 0, len(input.DomainIDs))
	requestDomains := make([]certrequest.DomainRef, 0, len(input.DomainIDs))
	hasWildcard := false
	for idx, domainID := range input.DomainIDs {
		asset, getErr := s.Domains.Get(scope, domainID)
		if getErr != nil {
			return certrequest.CreateInput{}, nil, getErr
		}

		relationType := certrequest.RelationSAN
		if idx == 0 {
			relationType = certrequest.RelationPrimary
		}
		if asset.AllowWildcard || strings.HasPrefix(asset.DomainName, "*.") {
			relationType = certrequest.RelationWildcard
			hasWildcard = true
		}

		resolved = append(resolved, asset)
		requestDomains = append(requestDomains, certrequest.DomainRef{
			DomainID:      asset.ID,
			RelationType:  relationType,
			SortOrder:     idx + 1,
			DomainName:    asset.DomainName,
			AllowWildcard: asset.AllowWildcard,
		})
	}

	requestType := strings.ToLower(strings.TrimSpace(input.RequestType))
	if requestType == "" {
		requestType = certrequest.RequestTypeIssue
	}

	certificateType := strings.ToLower(strings.TrimSpace(input.CertificateType))
	if certificateType == "" {
		switch {
		case hasWildcard:
			certificateType = certrequest.CertificateTypeWildcard
		case len(requestDomains) == 1:
			certificateType = certrequest.CertificateTypeSingle
		default:
			certificateType = certrequest.CertificateTypeSAN
		}
	}

	challengeType := strings.ToLower(strings.TrimSpace(input.ChallengeType))
	if challengeType == "" {
		challengeType = resolved[0].DefaultChallengeType
	}

	return certrequest.CreateInput{
		RequestType:     requestType,
		RequestSource:   certrequest.RequestSourceManual,
		AssetID:         strings.TrimSpace(input.AssetID),
		CAAccountID:     account.ID,
		CertificateType: certificateType,
		ChallengeType:   challengeType,
		CommonName:      resolved[0].DomainName,
		RequestedBy:     strings.TrimSpace(actorID),
		IdempotencyKey:  strings.TrimSpace(input.IdempotencyKey),
		Domains:         requestDomains,
	}, resolved, nil
}

func (s *Service) processStartJob(ctx context.Context, record job.Job) ProcessResult {
	scope := scopeFromJob(record)
	payload, err := parseStartPayload(record.Payload)
	if err != nil {
		return ProcessResult{ErrorCode: "INVALID_JOB_PAYLOAD", ErrorMessage: err.Error()}
	}

	request, err := s.Requests.Get(scope, payload.RequestID)
	if err != nil {
		return ProcessResult{ErrorCode: "REQUEST_NOT_FOUND", ErrorMessage: err.Error()}
	}
	if _, err := s.ensureRequestRunning(scope, request); err != nil {
		return ProcessResult{ErrorCode: "REQUEST_STATE_INVALID", ErrorMessage: err.Error()}
	}

	workflowRecord, workflowCreated, err := s.ensureWorkflow(scope, request)
	if err != nil {
		return ProcessResult{ErrorCode: "WORKFLOW_CREATE_FAILED", ErrorMessage: err.Error()}
	}
	if !workflowCreated && workflowRecord.Status != issueflow.StatusCreated && workflowRecord.Status != issueflow.StatusOrderPending {
		if _, scheduleErr := s.ensureContinueJob(ctx, request, workflowRecord, workflowRecord.Status); scheduleErr != nil {
			return ProcessResult{ErrorCode: "WORKFLOW_SCHEDULE_FAILED", ErrorMessage: scheduleErr.Error()}
		}
		return ProcessResult{}
	}

	requestDomains, err := s.Requests.ListDomains(scope, request.ID)
	if err != nil {
		return ProcessResult{ErrorCode: "REQUEST_DOMAIN_LOOKUP_FAILED", ErrorMessage: err.Error()}
	}

	order, err := s.ACME.CreateOrder(ctx, acmedriver.OrderRequest{
		RequestID:         request.ID,
		CommonName:        request.CommonName,
		CertificateType:   request.CertificateType,
		ChallengeType:     request.ChallengeType,
		DomainIdentifiers: mapOrderIdentifiers(requestDomains),
	})
	if err != nil {
		return s.classifyAdapterFailure(scope, request, workflowRecord, "ACME_ORDER_FAILED", err)
	}

	workflowRecord, err = s.Workflows.RecordOrder(scope, workflowRecord.ID, order.OrderURL, order.FinalizeURL)
	if err != nil {
		return ProcessResult{ErrorCode: "WORKFLOW_ORDER_RECORD_FAILED", ErrorMessage: err.Error()}
	}

	challengeInputs := make([]issueflow.ChallengeInput, 0, len(order.Challenges))
	for _, challenge := range order.Challenges {
		challengeInputs = append(challengeInputs, issueflow.ChallengeInput{
			DomainAssetID:    challenge.DomainAssetID,
			ChallengeType:    challenge.ChallengeType,
			Identifier:       challenge.Identifier,
			Token:            challenge.Token,
			KeyAuthorization: challenge.KeyAuthorization,
			HTTPPath:         challenge.HTTPPath,
			DNSRecordName:    challenge.DNSRecordName,
			DNSRecordValue:   challenge.DNSRecordValue,
		})
	}

	if _, workflowRecord, err = s.Workflows.EnsureChallenges(scope, workflowRecord.ID, challengeInputs); err != nil {
		return ProcessResult{ErrorCode: "WORKFLOW_CHALLENGE_CREATE_FAILED", ErrorMessage: err.Error()}
	}
	if _, err := s.ensureContinueJob(ctx, request, workflowRecord, issueflow.StatusChallengePending); err != nil {
		return ProcessResult{ErrorCode: "WORKFLOW_SCHEDULE_FAILED", ErrorMessage: err.Error()}
	}

	return ProcessResult{}
}

func (s *Service) processContinueJob(ctx context.Context, record job.Job) ProcessResult {
	scope := scopeFromJob(record)
	payload, err := parseContinuePayload(record.Payload)
	if err != nil {
		return ProcessResult{ErrorCode: "INVALID_JOB_PAYLOAD", ErrorMessage: err.Error()}
	}

	request, err := s.Requests.Get(scope, payload.RequestID)
	if err != nil {
		return ProcessResult{ErrorCode: "REQUEST_NOT_FOUND", ErrorMessage: err.Error()}
	}
	workflowRecord, err := s.Workflows.Get(scope, payload.WorkflowID)
	if err != nil {
		return ProcessResult{ErrorCode: "WORKFLOW_NOT_FOUND", ErrorMessage: err.Error()}
	}
	challenges, err := s.Workflows.ListChallenges(scope, workflowRecord.ID)
	if err != nil {
		return ProcessResult{ErrorCode: "WORKFLOW_CHALLENGE_LOOKUP_FAILED", ErrorMessage: err.Error()}
	}

	switch workflowRecord.Status {
	case issueflow.StatusChallengePending:
		return s.continueChallengePending(ctx, scope, request, workflowRecord, challenges)
	case issueflow.StatusChallengeProcessing:
		return s.continueChallengeProcessing(ctx, scope, request, workflowRecord, challenges)
	case issueflow.StatusChallengeValid, issueflow.StatusFinalizing:
		return s.continueFinalizing(ctx, scope, request, workflowRecord)
	case issueflow.StatusIssued:
		return s.completeIssued(ctx, scope, request, workflowRecord)
	case issueflow.StatusFailed, issueflow.StatusCancelled:
		return ProcessResult{}
	default:
		return ProcessResult{
			ErrorCode:    "WORKFLOW_STATUS_UNSUPPORTED",
			ErrorMessage: fmt.Sprintf("unsupported workflow status %q", workflowRecord.Status),
		}
	}
}

func (s *Service) continueChallengePending(
	ctx context.Context,
	scope resource.Scope,
	request certrequest.Request,
	workflowRecord issueflow.Workflow,
	challenges []issueflow.Challenge,
) ProcessResult {
	allReady := true
	for _, challenge := range challenges {
		current := challenge
		switch current.Status {
		case issueflow.ChallengeStatusPending:
			if _, err := s.Workflows.UpdateChallengeStatus(scope, workflowRecord.ID, current.ID, issueflow.ChallengeStatusPresenting); err != nil {
				return ProcessResult{ErrorCode: "WORKFLOW_CHALLENGE_STATE_FAILED", ErrorMessage: err.Error()}
			}
			switch current.ChallengeType {
			case certrequest.ChallengeTypeHTTP01:
				if err := s.HTTP01.Present(ctx, HTTP01Material{
					Identifier:       current.Identifier,
					Token:            current.Token,
					KeyAuthorization: current.KeyAuthorization,
					Path:             current.HTTPPath,
				}); err != nil {
					return s.classifyAdapterFailure(scope, request, workflowRecord, "CHALLENGE_PRESENT_FAILED", err)
				}
				if _, err := s.Workflows.UpdateChallengeStatus(scope, workflowRecord.ID, current.ID, issueflow.ChallengeStatusPresented); err != nil {
					return ProcessResult{ErrorCode: "WORKFLOW_CHALLENGE_STATE_FAILED", ErrorMessage: err.Error()}
				}
				if _, err := s.Workflows.UpdateChallengeStatus(scope, workflowRecord.ID, current.ID, issueflow.ChallengeStatusReady); err != nil {
					return ProcessResult{ErrorCode: "WORKFLOW_CHALLENGE_STATE_FAILED", ErrorMessage: err.Error()}
				}
			case certrequest.ChallengeTypeDNS01:
				if err := s.DNS.PresentTXT(ctx, dnsdriver.PresentInput{
					RecordName:  current.DNSRecordName,
					RecordValue: current.DNSRecordValue,
				}); err != nil {
					return s.classifyAdapterFailure(scope, request, workflowRecord, "CHALLENGE_PRESENT_FAILED", err)
				}
				if _, err := s.Workflows.UpdateChallengeStatus(scope, workflowRecord.ID, current.ID, issueflow.ChallengeStatusPresented); err != nil {
					return ProcessResult{ErrorCode: "WORKFLOW_CHALLENGE_STATE_FAILED", ErrorMessage: err.Error()}
				}
				if _, err := s.Workflows.UpdateChallengeStatus(scope, workflowRecord.ID, current.ID, issueflow.ChallengeStatusPropagating); err != nil {
					return ProcessResult{ErrorCode: "WORKFLOW_CHALLENGE_STATE_FAILED", ErrorMessage: err.Error()}
				}
				allReady = false
			}
		case issueflow.ChallengeStatusPresented:
			if current.ChallengeType == certrequest.ChallengeTypeDNS01 {
				if _, err := s.Workflows.UpdateChallengeStatus(scope, workflowRecord.ID, current.ID, issueflow.ChallengeStatusPropagating); err != nil {
					return ProcessResult{ErrorCode: "WORKFLOW_CHALLENGE_STATE_FAILED", ErrorMessage: err.Error()}
				}
				allReady = false
			}
		case issueflow.ChallengeStatusPropagating:
			ready, err := s.DNS.CheckPropagation(ctx, dnsdriver.PropagationInput{
				RecordName:  current.DNSRecordName,
				RecordValue: current.DNSRecordValue,
			})
			if err != nil {
				return s.classifyAdapterFailure(scope, request, workflowRecord, "DNS_PROPAGATION_FAILED", err)
			}
			if !ready {
				allReady = false
				continue
			}
			if _, err := s.Workflows.UpdateChallengeStatus(scope, workflowRecord.ID, current.ID, issueflow.ChallengeStatusReady); err != nil {
				return ProcessResult{ErrorCode: "WORKFLOW_CHALLENGE_STATE_FAILED", ErrorMessage: err.Error()}
			}
		case issueflow.ChallengeStatusReady:
		default:
			return ProcessResult{
				ErrorCode:    "WORKFLOW_CHALLENGE_STATE_INVALID",
				ErrorMessage: fmt.Sprintf("unexpected challenge status %q", current.Status),
			}
		}
	}

	if !allReady {
		return ProcessResult{Retryable: true, ErrorCode: "CHALLENGE_NOT_READY", ErrorMessage: "challenge propagation still pending"}
	}

	workflowRecord, err := s.Workflows.Transition(scope, workflowRecord.ID, issueflow.StatusChallengeProcessing)
	if err != nil {
		return ProcessResult{ErrorCode: "WORKFLOW_STATE_FAILED", ErrorMessage: err.Error()}
	}
	if _, err := s.ensureContinueJob(ctx, request, workflowRecord, workflowRecord.Status); err != nil {
		return ProcessResult{ErrorCode: "WORKFLOW_SCHEDULE_FAILED", ErrorMessage: err.Error()}
	}
	return ProcessResult{}
}

func (s *Service) continueChallengeProcessing(
	ctx context.Context,
	scope resource.Scope,
	request certrequest.Request,
	workflowRecord issueflow.Workflow,
	challenges []issueflow.Challenge,
) ProcessResult {
	allValid := true
	for _, challenge := range challenges {
		current := challenge
		switch current.Status {
		case issueflow.ChallengeStatusReady:
			if err := s.ACME.NotifyChallengeReady(ctx, acmedriver.ChallengeReadyInput{
				OrderURL:      workflowRecord.OrderURL,
				Identifier:    current.Identifier,
				ChallengeType: current.ChallengeType,
			}); err != nil {
				return s.classifyAdapterFailure(scope, request, workflowRecord, "CHALLENGE_VERIFY_FAILED", err)
			}
			if _, err := s.Workflows.UpdateChallengeStatus(scope, workflowRecord.ID, current.ID, issueflow.ChallengeStatusVerifying); err != nil {
				return ProcessResult{ErrorCode: "WORKFLOW_CHALLENGE_STATE_FAILED", ErrorMessage: err.Error()}
			}
			allValid = false
		case issueflow.ChallengeStatusVerifying:
			result, err := s.ACME.PollAuthorization(ctx, acmedriver.AuthorizationInput{
				OrderURL:      workflowRecord.OrderURL,
				Identifier:    current.Identifier,
				ChallengeType: current.ChallengeType,
			})
			if err != nil {
				return s.classifyAdapterFailure(scope, request, workflowRecord, "CHALLENGE_VERIFY_FAILED", err)
			}
			switch result.Status {
			case acmedriver.AuthorizationPending:
				allValid = false
			case acmedriver.AuthorizationValid:
				if _, err := s.Workflows.UpdateChallengeStatus(scope, workflowRecord.ID, current.ID, issueflow.ChallengeStatusValid); err != nil {
					return ProcessResult{ErrorCode: "WORKFLOW_CHALLENGE_STATE_FAILED", ErrorMessage: err.Error()}
				}
			case acmedriver.AuthorizationInvalid:
				if _, err := s.Workflows.UpdateChallengeStatus(scope, workflowRecord.ID, current.ID, issueflow.ChallengeStatusInvalid); err != nil {
					return ProcessResult{ErrorCode: "WORKFLOW_CHALLENGE_STATE_FAILED", ErrorMessage: err.Error()}
				}
				if _, err := s.Workflows.UpdateChallengeStatus(scope, workflowRecord.ID, current.ID, issueflow.ChallengeStatusCleanupPending); err != nil {
					return ProcessResult{ErrorCode: "WORKFLOW_CHALLENGE_STATE_FAILED", ErrorMessage: err.Error()}
				}
				s.cleanupChallenge(ctx, current)
				if _, err := s.Workflows.MarkFailed(scope, workflowRecord.ID, "CHALLENGE_VERIFY_FAILED", "challenge verification failed"); err != nil {
					return ProcessResult{ErrorCode: "WORKFLOW_STATE_FAILED", ErrorMessage: err.Error()}
				}
				if _, err := s.ensureRequestFailed(scope, request); err != nil {
					return ProcessResult{ErrorCode: "REQUEST_STATE_INVALID", ErrorMessage: err.Error()}
				}
				return ProcessResult{}
			default:
				return ProcessResult{ErrorCode: "CHALLENGE_VERIFY_FAILED", ErrorMessage: "unknown authorization status"}
			}
		case issueflow.ChallengeStatusValid:
		default:
			return ProcessResult{
				ErrorCode:    "WORKFLOW_CHALLENGE_STATE_INVALID",
				ErrorMessage: fmt.Sprintf("unexpected challenge status %q", current.Status),
			}
		}
	}

	if !allValid {
		return ProcessResult{Retryable: true, ErrorCode: "CHALLENGE_VERIFY_PENDING", ErrorMessage: "challenge verification still pending"}
	}

	workflowRecord, err := s.Workflows.Transition(scope, workflowRecord.ID, issueflow.StatusChallengeValid)
	if err != nil {
		return ProcessResult{ErrorCode: "WORKFLOW_STATE_FAILED", ErrorMessage: err.Error()}
	}
	if _, err := s.ensureContinueJob(ctx, request, workflowRecord, workflowRecord.Status); err != nil {
		return ProcessResult{ErrorCode: "WORKFLOW_SCHEDULE_FAILED", ErrorMessage: err.Error()}
	}
	return ProcessResult{}
}

func (s *Service) continueFinalizing(
	ctx context.Context,
	scope resource.Scope,
	request certrequest.Request,
	workflowRecord issueflow.Workflow,
) ProcessResult {
	var err error
	if workflowRecord.Status == issueflow.StatusChallengeValid {
		workflowRecord, err = s.Workflows.Transition(scope, workflowRecord.ID, issueflow.StatusFinalizing)
		if err != nil {
			return ProcessResult{ErrorCode: "WORKFLOW_STATE_FAILED", ErrorMessage: err.Error()}
		}
	}

	if _, err := s.ACME.FinalizeOrder(ctx, acmedriver.FinalizeInput{OrderURL: workflowRecord.OrderURL}); err != nil {
		return s.classifyAdapterFailure(scope, request, workflowRecord, "ACME_FINALIZE_FAILED", err)
	}
	bundle, err := s.ACME.DownloadCertificate(ctx, acmedriver.FinalizeInput{OrderURL: workflowRecord.OrderURL})
	if err != nil {
		return s.classifyAdapterFailure(scope, request, workflowRecord, "ACME_DOWNLOAD_FAILED", err)
	}

	workflowRecord, err = s.Workflows.MarkIssued(scope, workflowRecord.ID, bundle.CertificateRef)
	if err != nil {
		return ProcessResult{ErrorCode: "WORKFLOW_STATE_FAILED", ErrorMessage: err.Error()}
	}
	if _, err := s.ensureContinueJob(ctx, request, workflowRecord, workflowRecord.Status); err != nil {
		return ProcessResult{ErrorCode: "WORKFLOW_SCHEDULE_FAILED", ErrorMessage: err.Error()}
	}
	return ProcessResult{}
}

func (s *Service) completeIssued(
	ctx context.Context,
	scope resource.Scope,
	request certrequest.Request,
	workflowRecord issueflow.Workflow,
) ProcessResult {
	requestDomains, err := s.Requests.ListDomains(scope, request.ID)
	if err != nil {
		return ProcessResult{Retryable: true, ErrorCode: "REQUEST_DOMAIN_LOOKUP_FAILED", ErrorMessage: err.Error()}
	}
	domainIDs := make([]string, 0, len(requestDomains))
	for _, ref := range requestDomains {
		domainIDs = append(domainIDs, ref.DomainID)
	}

	asset, _, _, err := s.Assets.UpsertIssued(scope, certasset.IssueInput{
		AssetID:              request.AssetID,
		Name:                 request.CommonName,
		CAAccountID:          request.CAAccountID,
		CertificateType:      request.CertificateType,
		ChallengeType:        request.ChallengeType,
		CommonName:           request.CommonName,
		DomainIDs:            domainIDs,
		CertificateRequestID: request.ID,
		IssueWorkflowID:      workflowRecord.ID,
		CertificateRef:       workflowRecord.CertificateRef,
	})
	if err != nil {
		_, _ = s.Workflows.RecordError(scope, workflowRecord.ID, "ASSET_PERSIST_FAILED", err.Error())
		return ProcessResult{Retryable: true, ErrorCode: "ASSET_PERSIST_FAILED", ErrorMessage: err.Error()}
	}

	for _, domainID := range domainIDs {
		if err := s.Domains.LinkCertificateAsset(scope, domainID, domains.CertificateAssetRef{
			ID:     asset.ID,
			Name:   asset.Name,
			Status: asset.Status,
		}); err != nil {
			_, _ = s.Workflows.RecordError(scope, workflowRecord.ID, "ASSET_LINK_FAILED", err.Error())
			return ProcessResult{Retryable: true, ErrorCode: "ASSET_LINK_FAILED", ErrorMessage: err.Error()}
		}
	}

	if _, err := s.ensureRequestCompleted(scope, request); err != nil {
		return ProcessResult{ErrorCode: "REQUEST_STATE_INVALID", ErrorMessage: err.Error()}
	}
	return ProcessResult{}
}

func (s *Service) classifyAdapterFailure(scope resource.Scope, request certrequest.Request, workflowRecord issueflow.Workflow, code string, err error) ProcessResult {
	_, _ = s.Workflows.RecordError(scope, workflowRecord.ID, code, err.Error())
	switch {
	case acmedriver.IsTemporary(err), dnsdriver.IsTemporary(err):
		return ProcessResult{Retryable: true, ErrorCode: code, ErrorMessage: err.Error()}
	default:
		_, _ = s.Workflows.MarkFailed(scope, workflowRecord.ID, code, err.Error())
		_, _ = s.ensureRequestFailed(scope, request)
		return ProcessResult{}
	}
}

func (s *Service) cleanupChallenge(ctx context.Context, challenge issueflow.Challenge) {
	scope := challenge.Scope
	switch challenge.ChallengeType {
	case certrequest.ChallengeTypeDNS01:
		if err := s.DNS.CleanupTXT(ctx, dnsdriver.CleanupInput{
			RecordName:  challenge.DNSRecordName,
			RecordValue: challenge.DNSRecordValue,
		}); err != nil {
			_, _ = s.Workflows.UpdateChallengeStatus(scope, challenge.IssueWorkflowID, challenge.ID, issueflow.ChallengeStatusCleanupFailed)
			return
		}
	case certrequest.ChallengeTypeHTTP01:
		if err := s.HTTP01.Cleanup(ctx, HTTP01Material{
			Identifier:       challenge.Identifier,
			Token:            challenge.Token,
			KeyAuthorization: challenge.KeyAuthorization,
			Path:             challenge.HTTPPath,
		}); err != nil {
			_, _ = s.Workflows.UpdateChallengeStatus(scope, challenge.IssueWorkflowID, challenge.ID, issueflow.ChallengeStatusCleanupFailed)
			return
		}
	}

	_, _ = s.Workflows.UpdateChallengeStatus(scope, challenge.IssueWorkflowID, challenge.ID, issueflow.ChallengeStatusCleaned)
}

func (s *Service) ensureWorkflow(scope resource.Scope, request certrequest.Request) (issueflow.Workflow, bool, error) {
	record, err := s.Workflows.Create(scope, issueflow.CreateInput{
		CertificateRequestID: request.ID,
		CAAccountID:          request.CAAccountID,
		WorkflowType:         request.RequestType,
	})
	if errors.Is(err, resource.ErrConflict) {
		existing, lookupErr := s.Workflows.GetByRequest(scope, request.ID)
		return existing, false, lookupErr
	}
	if err != nil {
		return issueflow.Workflow{}, false, err
	}
	return record, true, nil
}

func (s *Service) ensureStartJob(ctx context.Context, request certrequest.Request) (job.Job, error) {
	key := startJobIdempotencyKey(request.ID)
	payload, _ := json.Marshal(startWorkflowPayload{RequestID: request.ID})
	record, err := s.Jobs.Schedule(ctx, jobscmd.ScheduleInput{
		TenantID:       request.Scope.TenantID,
		ProjectID:      request.Scope.ProjectID,
		EnvironmentID:  request.Scope.EnvironmentID,
		JobType:        jobTypeStartIssueWorkflow,
		AggregateType:  "certificate_request",
		AggregateID:    request.ID,
		Priority:       100,
		Payload:        payload,
		IdempotencyKey: key,
		Now:            s.now(),
	})
	if errors.Is(err, job.ErrDuplicateJob) {
		return s.Jobs.GetByIdempotency(ctx, key)
	}
	return record, err
}

func (s *Service) ensureContinueJob(ctx context.Context, request certrequest.Request, workflowRecord issueflow.Workflow, status string) (job.Job, error) {
	key := continueJobIdempotencyKey(workflowRecord.ID, status)
	payload, _ := json.Marshal(continueWorkflowPayload{
		RequestID:      request.ID,
		WorkflowID:     workflowRecord.ID,
		ExpectedStatus: status,
		Trigger:        request.RequestSource,
	})
	record, err := s.Jobs.Schedule(ctx, jobscmd.ScheduleInput{
		TenantID:       request.Scope.TenantID,
		ProjectID:      request.Scope.ProjectID,
		EnvironmentID:  request.Scope.EnvironmentID,
		JobType:        jobTypeContinueIssueWorkflow,
		AggregateType:  "issue_workflow",
		AggregateID:    workflowRecord.ID,
		Priority:       90,
		Payload:        payload,
		IdempotencyKey: key,
		Now:            s.now(),
	})
	if errors.Is(err, job.ErrDuplicateJob) {
		return s.Jobs.GetByIdempotency(ctx, key)
	}
	return record, err
}

func (s *Service) ensureRequestAccepted(scope resource.Scope, request certrequest.Request) (certrequest.Request, error) {
	switch request.Status {
	case certrequest.StatusAccepted, certrequest.StatusRunning, certrequest.StatusCompleted:
		return request, nil
	default:
		return s.Requests.MarkAccepted(scope, request.ID)
	}
}

func (s *Service) ensureRequestRunning(scope resource.Scope, request certrequest.Request) (certrequest.Request, error) {
	switch request.Status {
	case certrequest.StatusRunning, certrequest.StatusCompleted:
		return request, nil
	case certrequest.StatusSubmitted:
		if _, err := s.Requests.MarkAccepted(scope, request.ID); err != nil {
			return certrequest.Request{}, err
		}
		return s.Requests.MarkRunning(scope, request.ID)
	default:
		return s.Requests.MarkRunning(scope, request.ID)
	}
}

func (s *Service) ensureRequestCompleted(scope resource.Scope, request certrequest.Request) (certrequest.Request, error) {
	switch request.Status {
	case certrequest.StatusCompleted:
		return request, nil
	default:
		return s.Requests.MarkCompleted(scope, request.ID)
	}
}

func (s *Service) ensureRequestFailed(scope resource.Scope, request certrequest.Request) (certrequest.Request, error) {
	switch request.Status {
	case certrequest.StatusFailed:
		return request, nil
	case certrequest.StatusCompleted:
		return request, nil
	default:
		return s.Requests.MarkFailed(scope, request.ID)
	}
}

func scopeFromJob(record job.Job) resource.Scope {
	return resource.Scope{
		TenantID:      record.TenantID,
		ProjectID:     record.ProjectID,
		EnvironmentID: record.EnvironmentID,
	}
}

func parseStartPayload(raw json.RawMessage) (startWorkflowPayload, error) {
	var payload startWorkflowPayload
	if err := json.Unmarshal(raw, &payload); err != nil {
		return startWorkflowPayload{}, fmt.Errorf("decode start workflow payload: %w", err)
	}
	if strings.TrimSpace(payload.RequestID) == "" {
		return startWorkflowPayload{}, fmt.Errorf("request_id required: %w", resource.ErrValidation)
	}
	return payload, nil
}

func parseContinuePayload(raw json.RawMessage) (continueWorkflowPayload, error) {
	var payload continueWorkflowPayload
	if err := json.Unmarshal(raw, &payload); err != nil {
		return continueWorkflowPayload{}, fmt.Errorf("decode continue workflow payload: %w", err)
	}
	if strings.TrimSpace(payload.RequestID) == "" || strings.TrimSpace(payload.WorkflowID) == "" {
		return continueWorkflowPayload{}, fmt.Errorf("request_id and workflow_id required: %w", resource.ErrValidation)
	}
	return payload, nil
}

func mapOrderIdentifiers(refs []certrequest.DomainRef) []acmedriver.DomainIdentifier {
	items := make([]acmedriver.DomainIdentifier, 0, len(refs))
	for _, ref := range refs {
		items = append(items, acmedriver.DomainIdentifier{
			DomainAssetID: ref.DomainID,
			Name:          ref.DomainName,
		})
	}
	return items
}

func startJobIdempotencyKey(requestID string) string {
	return "issuewf:start:" + strings.TrimSpace(requestID)
}

func continueJobIdempotencyKey(workflowID string, status string) string {
	return "issuewf:continue:" + strings.TrimSpace(workflowID) + ":" + strings.TrimSpace(status)
}
