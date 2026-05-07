package wiring

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"

	controlplanehttp "github.com/zaneway/AutoCertX/internal/app/controlplane/http"
	caaccountscmd "github.com/zaneway/AutoCertX/internal/application/command/caaccounts"
	certificateassetscmd "github.com/zaneway/AutoCertX/internal/application/command/certificateassets"
	domainscmd "github.com/zaneway/AutoCertX/internal/application/command/domains"
	jobscmd "github.com/zaneway/AutoCertX/internal/application/command/jobs"
	nodescmd "github.com/zaneway/AutoCertX/internal/application/command/nodes"
	settingscmd "github.com/zaneway/AutoCertX/internal/application/command/settings"
	targetscmd "github.com/zaneway/AutoCertX/internal/application/command/targets"
	auditquery "github.com/zaneway/AutoCertX/internal/application/query/audit"
	dashboardquery "github.com/zaneway/AutoCertX/internal/application/query/dashboard"
	domainsquery "github.com/zaneway/AutoCertX/internal/application/query/domains"
	jobsquery "github.com/zaneway/AutoCertX/internal/application/query/jobs"
	settingsquery "github.com/zaneway/AutoCertX/internal/application/query/settings"
	deploymentservice "github.com/zaneway/AutoCertX/internal/deployment"
	"github.com/zaneway/AutoCertX/internal/domain/agentnode"
	auditdomain "github.com/zaneway/AutoCertX/internal/domain/audit"
	certificateasset "github.com/zaneway/AutoCertX/internal/domain/certificateasset"
	certificaterequest "github.com/zaneway/AutoCertX/internal/domain/certificaterequest"
	"github.com/zaneway/AutoCertX/internal/domain/deploymentrecord"
	"github.com/zaneway/AutoCertX/internal/domain/deploymenttarget"
	"github.com/zaneway/AutoCertX/internal/domain/dnscredentials"
	"github.com/zaneway/AutoCertX/internal/domain/domains"
	"github.com/zaneway/AutoCertX/internal/domain/issuer"
	issueworkflow "github.com/zaneway/AutoCertX/internal/domain/issueworkflow"
	settingsdomain "github.com/zaneway/AutoCertX/internal/domain/settings"
	acmedriver "github.com/zaneway/AutoCertX/internal/driver/acme"
	agenttransportdriver "github.com/zaneway/AutoCertX/internal/driver/agenttransport"
	dnsdriver "github.com/zaneway/AutoCertX/internal/driver/dns"
	"github.com/zaneway/AutoCertX/internal/platform/buildinfo"
	"github.com/zaneway/AutoCertX/internal/platform/config"
	"github.com/zaneway/AutoCertX/internal/platform/logging"
	"github.com/zaneway/AutoCertX/internal/platform/runtime"
	workflowservice "github.com/zaneway/AutoCertX/internal/workflow"
)

// Options controls dependency construction for the control plane process.
type Options struct {
	ServiceName     string
	EnvPrefix       string
	DefaultHTTPAddr string
}

// Result is the assembled process dependency graph for the control plane.
type Result struct {
	Config config.Config
	Logger *slog.Logger
	Server *http.Server
}

// Build assembles configuration, logger, HTTP router and HTTP server.
func Build(opts Options) (Result, error) {
	cfg, err := config.Load(config.LoadOptions{
		ServiceName:     opts.ServiceName,
		EnvPrefix:       opts.EnvPrefix,
		DefaultHTTPAddr: opts.DefaultHTTPAddr,
	})
	if err != nil {
		return Result{}, fmt.Errorf("load config: %w", err)
	}

	logger, err := logging.New(cfg.LogLevel)
	if err != nil {
		return Result{}, fmt.Errorf("new logger: %w", err)
	}

	build := buildinfo.Current(cfg.ServiceName)
	authService, authContextService, err := newAuthServices(cfg)
	if err != nil {
		return Result{}, fmt.Errorf("build auth services: %w", err)
	}
	domainService := domains.NewService()
	dnsService := dnscredentials.NewService()
	issuerService := issuer.NewService()
	requestService := certificaterequest.NewService()
	assetService := certificateasset.NewService()
	workflowDomainService := issueworkflow.NewService()
	nodeService := agentnode.NewService()
	targetService := deploymenttarget.NewService()
	deploymentRecordService := deploymentrecord.NewService()
	auditService := auditdomain.NewService()
	settingsService := settingsdomain.NewService()
	jobRepo := jobscmd.NewMemoryRepository()
	jobsService, err := jobscmd.NewService(jobRepo, jobscmd.Options{})
	if err != nil {
		return Result{}, fmt.Errorf("build jobs service: %w", err)
	}
	workflowCommandService, err := workflowservice.NewService(
		requestService,
		workflowDomainService,
		assetService,
		domainService,
		issuerService,
		jobsService,
		acmedriver.NewFakeClient(),
		dnsdriver.NewFakeExecutor(),
		workflowservice.FakeHTTP01Presenter{},
	)
	if err != nil {
		return Result{}, fmt.Errorf("build workflow service: %w", err)
	}
	var deploymentService *deploymentservice.Service
	agentTransportService, err := agenttransportdriver.NewService(nodeService, agenttransportdriver.Options{
		ProgressCallback: func(ctx context.Context, req agenttransportdriver.JobProgressRequest) error {
			if deploymentService == nil {
				return nil
			}
			return deploymentService.HandleAgentProgress(ctx, req)
		},
		CompletionCallback: func(ctx context.Context, req agenttransportdriver.JobCompleteRequest) error {
			if deploymentService == nil {
				return nil
			}
			return deploymentService.HandleAgentComplete(ctx, req)
		},
	})
	if err != nil {
		return Result{}, fmt.Errorf("build agent transport service: %w", err)
	}
	deploymentService, err = deploymentservice.NewService(assetService, targetService, nodeService, deploymentRecordService, agentTransportService, auditService)
	if err != nil {
		return Result{}, fmt.Errorf("build deployment service: %w", err)
	}
	governanceQuery, err := domainsquery.NewService(domainService, dnsService, issuerService)
	if err != nil {
		return Result{}, fmt.Errorf("build governance query: %w", err)
	}
	settingsQuery, err := settingsquery.NewService(auditService, settingsService)
	if err != nil {
		return Result{}, fmt.Errorf("build settings query: %w", err)
	}
	jobsQuery, err := jobsquery.NewService(jobRepo)
	if err != nil {
		return Result{}, fmt.Errorf("build jobs query: %w", err)
	}
	dashboardQuery, err := dashboardquery.NewService(domainService, dnsService, issuerService, auditService, jobRepo)
	if err != nil {
		return Result{}, fmt.Errorf("build dashboard query: %w", err)
	}
	handler := controlplanehttp.NewRouter(controlplanehttp.Deps{
		Config:            cfg,
		BuildInfo:         build,
		Logger:            logger,
		AuthService:       authService,
		AuthContextQuery:  authContextService,
		AgentTransport:    agentTransportService,
		DeploymentService: deploymentService,
		CertificateAssets: certificateassetscmd.NewService(workflowCommandService),
		DomainCommands:    domainscmd.NewService(domainService, dnsService, domainsAuditRecorder{audit: auditService}),
		CAAccountCommands: caaccountscmd.NewService(issuerService),
		NodeCommands:      nodescmd.NewService(nodeService),
		TargetCommands:    targetscmd.NewService(targetService),
		SettingsCommands:  settingscmd.NewService(auditService, settingsService, ""),
		GovernanceQuery:   governanceQuery,
		SettingsQuery:     settingsQuery,
		JobsQuery:         jobsQuery,
		DashboardQuery:    dashboardQuery,
		AuditQuery:        auditquery.NewService(auditService),
	})

	return Result{
		Config: cfg,
		Logger: logger,
		Server: runtime.NewServer(cfg, handler),
	}, nil
}
