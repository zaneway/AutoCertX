package audit

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/url"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/zaneway/AutoCertX/internal/domain/resource"
	"github.com/zaneway/AutoCertX/internal/platform/uuidx"
)

const (
	ActorTypeUser   = "user"
	ActorTypeSystem = "system"
	ActorTypeAgent  = "agent"
	ActorTypeAPIKey = "api_key"

	WebhookStatusActive   = "active"
	WebhookStatusDisabled = "disabled"

	NotificationStatusPending    = "pending"
	NotificationStatusDelivering = "delivering"
	NotificationStatusSucceeded  = "succeeded"
	NotificationStatusFailed     = "failed"
	NotificationStatusRetry      = "retry"
	NotificationStatusCancelled  = "cancelled"

	EvidenceStatusActive   = "active"
	EvidenceStatusArchived = "archived"

	ExportTypeAudit    = "audit"
	ExportTypeEvidence = "evidence"
	ExportTypeReport   = "report"

	ExportFormatCSV  = "csv"
	ExportFormatPDF  = "pdf"
	ExportFormatJSON = "json"

	ExportStatusPending   = "pending"
	ExportStatusRunning   = "running"
	ExportStatusSucceeded = "succeeded"
	ExportStatusFailed    = "failed"
	ExportStatusExpired   = "expired"
)

// Event is the audit trail entity exposed to query/read APIs.
type Event struct {
	ID           string            `json:"id"`
	Scope        resource.Scope    `json:"-"`
	ActorType    string            `json:"actor_type"`
	ActorID      string            `json:"actor_id"`
	Action       string            `json:"action"`
	ResourceType string            `json:"resource_type"`
	ResourceID   string            `json:"resource_id,omitempty"`
	RequestID    string            `json:"request_id,omitempty"`
	TraceID      string            `json:"trace_id,omitempty"`
	Detail       map[string]string `json:"detail,omitempty"`
	OccurredAt   time.Time         `json:"occurred_at"`
	CreatedAt    time.Time         `json:"created_at"`
	UpdatedAt    time.Time         `json:"updated_at"`
}

// EventFilter narrows audit list/export operations.
type EventFilter struct {
	ActorID      string
	Action       string
	ResourceType string
	ResourceID   string
	StartTime    *time.Time
	EndTime      *time.Time
}

// EventInput is the write model for one audit event.
type EventInput struct {
	ActorType    string
	ActorID      string
	Action       string
	ResourceType string
	ResourceID   string
	RequestID    string
	TraceID      string
	Detail       map[string]string
	OccurredAt   time.Time
}

// EvidenceRecord tracks evidence material attached to auditable resources.
type EvidenceRecord struct {
	ID           string            `json:"id"`
	Scope        resource.Scope    `json:"-"`
	ResourceType string            `json:"resource_type"`
	ResourceID   string            `json:"resource_id"`
	EvidenceType string            `json:"evidence_type"`
	StorageRef   string            `json:"storage_ref"`
	DigestSHA256 string            `json:"digest_sha256"`
	Metadata     map[string]string `json:"metadata,omitempty"`
	Status       string            `json:"status"`
	CreatedAt    time.Time         `json:"created_at"`
	UpdatedAt    time.Time         `json:"updated_at"`
}

// EvidenceInput is the write model for evidence material.
type EvidenceInput struct {
	ResourceType string
	ResourceID   string
	EvidenceType string
	StorageRef   string
	DigestSHA256 string
	Metadata     map[string]string
}

// WebhookEndpoint configures one outbound notification sink.
type WebhookEndpoint struct {
	ID           string         `json:"id"`
	Scope        resource.Scope `json:"-"`
	Name         string         `json:"name"`
	URL          string         `json:"url"`
	SecretDigest string         `json:"-"`
	EventTypes   []string       `json:"event_types"`
	Status       string         `json:"status"`
	LastTestedAt *time.Time     `json:"last_tested_at,omitempty"`
	CreatedAt    time.Time      `json:"created_at"`
	UpdatedAt    time.Time      `json:"updated_at"`
}

// WebhookUpsertInput is the write model for webhook endpoint CRUD.
type WebhookUpsertInput struct {
	Name       string
	URL        string
	Secret     string
	EventTypes []string
	Enabled    bool
}

// NotificationEvent captures one delivery attempt lifecycle.
type NotificationEvent struct {
	ID                string            `json:"id"`
	Scope             resource.Scope    `json:"-"`
	WebhookEndpointID string            `json:"webhook_endpoint_id"`
	EventType         string            `json:"event_type"`
	ResourceType      string            `json:"resource_type"`
	ResourceID        string            `json:"resource_id,omitempty"`
	Payload           map[string]string `json:"payload,omitempty"`
	Status            string            `json:"status"`
	AttemptCount      int               `json:"attempt_count"`
	LastErrorCode     string            `json:"last_error_code,omitempty"`
	LastErrorMessage  string            `json:"last_error_message,omitempty"`
	NextRetryAt       *time.Time        `json:"next_retry_at,omitempty"`
	DeliveredAt       *time.Time        `json:"delivered_at,omitempty"`
	CreatedAt         time.Time         `json:"created_at"`
	UpdatedAt         time.Time         `json:"updated_at"`
}

// ExportRecord tracks one generated export artifact.
type ExportRecord struct {
	ID           string            `json:"id"`
	Scope        resource.Scope    `json:"-"`
	ExportNo     string            `json:"export_no"`
	ExportType   string            `json:"export_type"`
	ResourceType string            `json:"resource_type,omitempty"`
	RequestedBy  string            `json:"requested_by,omitempty"`
	Format       string            `json:"format"`
	Filters      map[string]string `json:"filters,omitempty"`
	Status       string            `json:"status"`
	StorageRef   string            `json:"storage_ref,omitempty"`
	ErrorCode    string            `json:"error_code,omitempty"`
	ErrorMessage string            `json:"error_message,omitempty"`
	FinishedAt   *time.Time        `json:"finished_at,omitempty"`
	CreatedAt    time.Time         `json:"created_at"`
	UpdatedAt    time.Time         `json:"updated_at"`
}

// ExportInput is the write model for export tracking.
type ExportInput struct {
	ExportType   string
	ResourceType string
	RequestedBy  string
	Format       string
	Filters      map[string]string
	Status       string
}

// Deliverer abstracts webhook delivery.
type Deliverer interface {
	Deliver(context.Context, WebhookEndpoint, NotificationEvent) error
}

// Service owns in-memory T06 audit/webhook/export state.
type Service struct {
	mu                    sync.RWMutex
	now                   func() time.Time
	newID                 func() string
	newExportNo           func() string
	maxDeliveryAttempts   int
	retryBaseInterval     time.Duration
	deliverer             Deliverer
	eventsByID            map[string]Event
	evidenceByID          map[string]EvidenceRecord
	webhooksByID          map[string]WebhookEndpoint
	webhookNameByEnvScope map[string]string
	notificationsByID     map[string]NotificationEvent
	exportsByID           map[string]ExportRecord
}

// NewService constructs the in-memory audit service.
func NewService() *Service {
	return &Service{
		now: func() time.Time {
			return time.Now().UTC()
		},
		newID: func() string {
			return uuidx.New()
		},
		newExportNo: func() string {
			return "EXP-" + strings.ToUpper(strings.ReplaceAll(uuidx.New(), "-", ""))
		},
		maxDeliveryAttempts:   3,
		retryBaseInterval:     5 * time.Minute,
		deliverer:             stubDeliverer{},
		eventsByID:            make(map[string]Event),
		evidenceByID:          make(map[string]EvidenceRecord),
		webhooksByID:          make(map[string]WebhookEndpoint),
		webhookNameByEnvScope: make(map[string]string),
		notificationsByID:     make(map[string]NotificationEvent),
		exportsByID:           make(map[string]ExportRecord),
	}
}

// RecordEvent appends one audit event and fans out matching webhook notifications.
func (s *Service) RecordEvent(ctx context.Context, scope resource.Scope, input EventInput) (Event, error) {
	if err := scope.Validate(); err != nil {
		return Event{}, err
	}

	event, err := buildEvent(s.newID, s.now, scope, input)
	if err != nil {
		return Event{}, err
	}

	notificationIDs := make([]string, 0)

	s.mu.Lock()
	s.eventsByID[event.ID] = event

	// Fan out matching webhook notifications while holding the lock only long
	// enough to snapshot the delivery work to be processed afterward.
	for _, endpoint := range s.webhooksByID {
		if !endpoint.Scope.Equals(scope) || endpoint.Status != WebhookStatusActive {
			continue
		}
		if !matchesEventType(endpoint.EventTypes, event.Action) {
			continue
		}

		notification := NotificationEvent{
			ID:                s.newID(),
			Scope:             scope,
			WebhookEndpointID: endpoint.ID,
			EventType:         event.Action,
			ResourceType:      event.ResourceType,
			ResourceID:        event.ResourceID,
			Payload: map[string]string{
				"audit_event_id": event.ID,
				"actor_id":       event.ActorID,
				"action":         event.Action,
				"resource_type":  event.ResourceType,
				"resource_id":    event.ResourceID,
			},
			Status:       NotificationStatusPending,
			AttemptCount: 0,
			CreatedAt:    event.CreatedAt,
			UpdatedAt:    event.CreatedAt,
		}
		s.notificationsByID[notification.ID] = notification
		notificationIDs = append(notificationIDs, notification.ID)
	}
	s.mu.Unlock()

	for _, notificationID := range notificationIDs {
		// Delivery is attempted after the event is durably visible so query APIs can
		// still inspect the audit record even if webhook delivery fails.
		_ = s.processNotification(ctx, notificationID)
	}

	return event, nil
}

// ListEvents returns audit events under the caller scope.
func (s *Service) ListEvents(scope resource.Scope, filter EventFilter) ([]Event, error) {
	if err := scope.Validate(); err != nil {
		return nil, err
	}
	if err := validateEventFilter(filter); err != nil {
		return nil, err
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	items := make([]Event, 0)
	for _, event := range s.eventsByID {
		if !event.Scope.Equals(scope) || !filterMatchesEvent(filter, event) {
			continue
		}
		items = append(items, cloneEvent(event))
	}

	slices.SortFunc(items, compareEvents)
	return items, nil
}

// GetEvent returns one audit event under the caller scope.
func (s *Service) GetEvent(scope resource.Scope, id string) (Event, error) {
	if err := scope.Validate(); err != nil {
		return Event{}, err
	}
	if strings.TrimSpace(id) == "" {
		return Event{}, fmt.Errorf("audit event id required: %w", resource.ErrValidation)
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	event, ok := s.eventsByID[id]
	if !ok {
		return Event{}, fmt.Errorf("audit event %s: %w", id, resource.ErrNotFound)
	}
	if !event.Scope.Equals(scope) {
		return Event{}, fmt.Errorf("audit event %s: %w", id, resource.ErrScopeMismatch)
	}
	return cloneEvent(event), nil
}

// AddEvidence stores one evidence record.
func (s *Service) AddEvidence(scope resource.Scope, input EvidenceInput) (EvidenceRecord, error) {
	if err := scope.Validate(); err != nil {
		return EvidenceRecord{}, err
	}
	if err := validateEvidenceInput(input); err != nil {
		return EvidenceRecord{}, err
	}

	now := s.now()
	record := EvidenceRecord{
		ID:           s.newID(),
		Scope:        scope,
		ResourceType: strings.TrimSpace(input.ResourceType),
		ResourceID:   strings.TrimSpace(input.ResourceID),
		EvidenceType: strings.TrimSpace(input.EvidenceType),
		StorageRef:   strings.TrimSpace(input.StorageRef),
		DigestSHA256: strings.TrimSpace(input.DigestSHA256),
		Metadata:     cloneMap(input.Metadata),
		Status:       EvidenceStatusActive,
		CreatedAt:    now,
		UpdatedAt:    now,
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.evidenceByID[record.ID] = record
	return record, nil
}

// CreateWebhookEndpoint creates one active/disabled webhook endpoint under scope.
func (s *Service) CreateWebhookEndpoint(scope resource.Scope, input WebhookUpsertInput) (WebhookEndpoint, error) {
	if err := scope.Validate(); err != nil {
		return WebhookEndpoint{}, err
	}

	normalized, err := normalizeWebhookInput(input, true)
	if err != nil {
		return WebhookEndpoint{}, err
	}

	now := s.now()
	endpoint := WebhookEndpoint{
		ID:           s.newID(),
		Scope:        scope,
		Name:         normalized.Name,
		URL:          normalized.URL,
		SecretDigest: digestSecret(normalized.Secret),
		EventTypes:   normalized.EventTypes,
		Status:       normalized.Status,
		CreatedAt:    now,
		UpdatedAt:    now,
	}

	scopeKey := scope.EnvironmentKey() + "/" + endpoint.Name

	s.mu.Lock()
	defer s.mu.Unlock()

	// Endpoint names are unique per environment so operators can use stable,
	// human-readable identifiers without accidental duplicates.
	if existingID, ok := s.webhookNameByEnvScope[scopeKey]; ok {
		return WebhookEndpoint{}, fmt.Errorf("webhook endpoint %q already exists under environment (%s): %w", endpoint.Name, existingID, resource.ErrConflict)
	}
	s.webhooksByID[endpoint.ID] = endpoint
	s.webhookNameByEnvScope[scopeKey] = endpoint.ID
	return cloneWebhookEndpoint(endpoint), nil
}

// UpdateWebhookEndpoint updates one existing webhook endpoint under scope.
func (s *Service) UpdateWebhookEndpoint(scope resource.Scope, id string, input WebhookUpsertInput) (WebhookEndpoint, error) {
	if err := scope.Validate(); err != nil {
		return WebhookEndpoint{}, err
	}
	if strings.TrimSpace(id) == "" {
		return WebhookEndpoint{}, fmt.Errorf("webhook endpoint id required: %w", resource.ErrValidation)
	}

	normalized, err := normalizeWebhookInput(input, false)
	if err != nil {
		return WebhookEndpoint{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	endpoint, ok := s.webhooksByID[id]
	if !ok {
		return WebhookEndpoint{}, fmt.Errorf("webhook endpoint %s: %w", id, resource.ErrNotFound)
	}
	if !endpoint.Scope.Equals(scope) {
		return WebhookEndpoint{}, fmt.Errorf("webhook endpoint %s: %w", id, resource.ErrScopeMismatch)
	}

	newName := normalized.Name
	oldKey := endpoint.Scope.EnvironmentKey() + "/" + endpoint.Name
	newKey := endpoint.Scope.EnvironmentKey() + "/" + newName
	if oldKey != newKey {
		// Renames must preserve the same uniqueness invariant as creates.
		if existingID, ok := s.webhookNameByEnvScope[newKey]; ok && existingID != endpoint.ID {
			return WebhookEndpoint{}, fmt.Errorf("webhook endpoint %q already exists under environment (%s): %w", newName, existingID, resource.ErrConflict)
		}
		delete(s.webhookNameByEnvScope, oldKey)
		s.webhookNameByEnvScope[newKey] = endpoint.ID
	}

	endpoint.Name = newName
	endpoint.URL = normalized.URL
	if normalized.Secret != "" {
		endpoint.SecretDigest = digestSecret(normalized.Secret)
	}
	endpoint.EventTypes = normalized.EventTypes
	endpoint.Status = normalized.Status
	endpoint.UpdatedAt = s.now()
	s.webhooksByID[endpoint.ID] = endpoint
	return cloneWebhookEndpoint(endpoint), nil
}

// ListWebhookEndpoints returns webhook endpoints under the given scope.
func (s *Service) ListWebhookEndpoints(scope resource.Scope) ([]WebhookEndpoint, error) {
	if err := scope.Validate(); err != nil {
		return nil, err
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	items := make([]WebhookEndpoint, 0)
	for _, endpoint := range s.webhooksByID {
		if !endpoint.Scope.Equals(scope) {
			continue
		}
		items = append(items, cloneWebhookEndpoint(endpoint))
	}

	slices.SortFunc(items, func(a WebhookEndpoint, b WebhookEndpoint) int {
		if a.Name == b.Name {
			return strings.Compare(a.ID, b.ID)
		}
		return strings.Compare(a.Name, b.Name)
	})
	return items, nil
}

// ListNotificationEvents returns outbound notifications for one scope.
func (s *Service) ListNotificationEvents(scope resource.Scope) ([]NotificationEvent, error) {
	if err := scope.Validate(); err != nil {
		return nil, err
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	items := make([]NotificationEvent, 0)
	for _, notification := range s.notificationsByID {
		if !notification.Scope.Equals(scope) {
			continue
		}
		items = append(items, cloneNotification(notification))
	}

	slices.SortFunc(items, func(a NotificationEvent, b NotificationEvent) int {
		if a.CreatedAt.Equal(b.CreatedAt) {
			return strings.Compare(a.ID, b.ID)
		}
		if a.CreatedAt.After(b.CreatedAt) {
			return -1
		}
		return 1
	})
	return items, nil
}

// ProcessDueNotifications retries notifications already scheduled for delivery.
func (s *Service) ProcessDueNotifications(ctx context.Context) error {
	now := s.now()

	s.mu.RLock()
	ids := make([]string, 0)
	for id, notification := range s.notificationsByID {
		switch notification.Status {
		case NotificationStatusPending:
			ids = append(ids, id)
		case NotificationStatusRetry:
			if notification.NextRetryAt != nil && !notification.NextRetryAt.After(now) {
				ids = append(ids, id)
			}
		}
	}
	s.mu.RUnlock()

	for _, id := range ids {
		// Each notification is retried independently so one broken endpoint does
		// not block the rest of the backlog.
		if err := s.processNotification(ctx, id); err != nil {
			return err
		}
	}
	return nil
}

// CreateExportRecord starts tracking one export artifact.
func (s *Service) CreateExportRecord(scope resource.Scope, input ExportInput) (ExportRecord, error) {
	if err := scope.Validate(); err != nil {
		return ExportRecord{}, err
	}
	if err := validateExportInput(input); err != nil {
		return ExportRecord{}, err
	}

	now := s.now()
	record := ExportRecord{
		ID:           s.newID(),
		Scope:        scope,
		ExportNo:     s.newExportNo(),
		ExportType:   strings.TrimSpace(input.ExportType),
		ResourceType: strings.TrimSpace(input.ResourceType),
		RequestedBy:  strings.TrimSpace(input.RequestedBy),
		Format:       strings.TrimSpace(input.Format),
		Filters:      cloneMap(input.Filters),
		Status:       strings.TrimSpace(input.Status),
		CreatedAt:    now,
		UpdatedAt:    now,
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	// Export tracking is created before artifact generation so failures can still
	// be surfaced as a first-class export record.
	s.exportsByID[record.ID] = record
	return cloneExportRecord(record), nil
}

// MarkExportSucceeded transitions one export record into succeeded state.
func (s *Service) MarkExportSucceeded(scope resource.Scope, id string, storageRef string) (ExportRecord, error) {
	if err := scope.Validate(); err != nil {
		return ExportRecord{}, err
	}
	if strings.TrimSpace(id) == "" {
		return ExportRecord{}, fmt.Errorf("export record id required: %w", resource.ErrValidation)
	}
	if strings.TrimSpace(storageRef) == "" {
		return ExportRecord{}, fmt.Errorf("storage_ref required: %w", resource.ErrValidation)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	record, ok := s.exportsByID[id]
	if !ok {
		return ExportRecord{}, fmt.Errorf("export record %s: %w", id, resource.ErrNotFound)
	}
	if !record.Scope.Equals(scope) {
		return ExportRecord{}, fmt.Errorf("export record %s: %w", id, resource.ErrScopeMismatch)
	}

	now := s.now()
	record.Status = ExportStatusSucceeded
	record.StorageRef = strings.TrimSpace(storageRef)
	record.ErrorCode = ""
	record.ErrorMessage = ""
	record.FinishedAt = &now
	record.UpdatedAt = now
	s.exportsByID[record.ID] = record
	return cloneExportRecord(record), nil
}

// MarkExportFailed transitions one export record into failed state.
func (s *Service) MarkExportFailed(scope resource.Scope, id string, code string, message string) (ExportRecord, error) {
	if err := scope.Validate(); err != nil {
		return ExportRecord{}, err
	}
	if strings.TrimSpace(id) == "" {
		return ExportRecord{}, fmt.Errorf("export record id required: %w", resource.ErrValidation)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	record, ok := s.exportsByID[id]
	if !ok {
		return ExportRecord{}, fmt.Errorf("export record %s: %w", id, resource.ErrNotFound)
	}
	if !record.Scope.Equals(scope) {
		return ExportRecord{}, fmt.Errorf("export record %s: %w", id, resource.ErrScopeMismatch)
	}

	now := s.now()
	record.Status = ExportStatusFailed
	record.ErrorCode = strings.TrimSpace(code)
	record.ErrorMessage = strings.TrimSpace(message)
	record.FinishedAt = &now
	record.UpdatedAt = now
	s.exportsByID[record.ID] = record
	return cloneExportRecord(record), nil
}

// GetExportRecord returns one export record under the caller scope.
func (s *Service) GetExportRecord(scope resource.Scope, id string) (ExportRecord, error) {
	if err := scope.Validate(); err != nil {
		return ExportRecord{}, err
	}
	if strings.TrimSpace(id) == "" {
		return ExportRecord{}, fmt.Errorf("export record id required: %w", resource.ErrValidation)
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	record, ok := s.exportsByID[id]
	if !ok {
		return ExportRecord{}, fmt.Errorf("export record %s: %w", id, resource.ErrNotFound)
	}
	if !record.Scope.Equals(scope) {
		return ExportRecord{}, fmt.Errorf("export record %s: %w", id, resource.ErrScopeMismatch)
	}
	return cloneExportRecord(record), nil
}

func (s *Service) processNotification(ctx context.Context, id string) error {
	s.mu.Lock()
	notification, ok := s.notificationsByID[id]
	if !ok {
		s.mu.Unlock()
		return fmt.Errorf("notification event %s: %w", id, resource.ErrNotFound)
	}
	endpoint, ok := s.webhooksByID[notification.WebhookEndpointID]
	if !ok {
		s.mu.Unlock()
		return fmt.Errorf("webhook endpoint %s: %w", notification.WebhookEndpointID, resource.ErrNotFound)
	}
	if !notification.Scope.Equals(endpoint.Scope) {
		s.mu.Unlock()
		return fmt.Errorf("notification endpoint scope mismatch: %w", resource.ErrScopeMismatch)
	}

	now := s.now()
	// Move to delivering before the network call so concurrent processors observe
	// that this notification is already in-flight.
	notification.Status = NotificationStatusDelivering
	notification.AttemptCount++
	notification.UpdatedAt = now
	notification.NextRetryAt = nil
	s.notificationsByID[notification.ID] = notification
	s.mu.Unlock()

	err := s.deliverer.Deliver(ctx, endpoint, notification)

	s.mu.Lock()
	defer s.mu.Unlock()

	current := s.notificationsByID[notification.ID]
	now = s.now()
	current.UpdatedAt = now
	if err == nil {
		// Success clears retry bookkeeping and stamps the delivery completion time.
		current.Status = NotificationStatusSucceeded
		current.LastErrorCode = ""
		current.LastErrorMessage = ""
		current.DeliveredAt = &now
		s.notificationsByID[current.ID] = current
		return nil
	}

	current.LastErrorCode = "WEBHOOK_DELIVERY_FAILED"
	current.LastErrorMessage = err.Error()
	if current.AttemptCount >= s.maxDeliveryAttempts {
		// Exhausted notifications stop retrying but remain queryable for operator
		// troubleshooting.
		current.Status = NotificationStatusFailed
	} else {
		// Retry delay grows with attempt count to avoid hot-looping broken endpoints.
		current.Status = NotificationStatusRetry
		retryAt := now.Add(time.Duration(current.AttemptCount) * s.retryBaseInterval)
		current.NextRetryAt = &retryAt
	}
	s.notificationsByID[current.ID] = current
	return nil
}

func buildEvent(newID func() string, now func() time.Time, scope resource.Scope, input EventInput) (Event, error) {
	actorType := strings.TrimSpace(input.ActorType)
	if actorType == "" {
		// System is the default actor type so background actions remain auditable
		// even when no human principal is attached.
		actorType = ActorTypeSystem
	}
	switch actorType {
	case ActorTypeUser, ActorTypeSystem, ActorTypeAgent, ActorTypeAPIKey:
	default:
		return Event{}, fmt.Errorf("unsupported actor_type %q: %w", input.ActorType, resource.ErrValidation)
	}
	if strings.TrimSpace(input.ActorID) == "" {
		return Event{}, fmt.Errorf("actor_id required: %w", resource.ErrValidation)
	}
	if strings.TrimSpace(input.Action) == "" {
		return Event{}, fmt.Errorf("action required: %w", resource.ErrValidation)
	}
	if strings.TrimSpace(input.ResourceType) == "" {
		return Event{}, fmt.Errorf("resource_type required: %w", resource.ErrValidation)
	}

	occurredAt := input.OccurredAt
	if occurredAt.IsZero() {
		occurredAt = now()
	}

	createdAt := now()
	return Event{
		ID:           newID(),
		Scope:        scope,
		ActorType:    actorType,
		ActorID:      strings.TrimSpace(input.ActorID),
		Action:       strings.TrimSpace(input.Action),
		ResourceType: strings.TrimSpace(input.ResourceType),
		ResourceID:   strings.TrimSpace(input.ResourceID),
		RequestID:    strings.TrimSpace(input.RequestID),
		TraceID:      strings.TrimSpace(input.TraceID),
		Detail:       cloneMap(input.Detail),
		OccurredAt:   occurredAt.UTC(),
		CreatedAt:    createdAt,
		UpdatedAt:    createdAt,
	}, nil
}

func validateEventFilter(filter EventFilter) error {
	if filter.StartTime != nil && filter.EndTime != nil && filter.EndTime.Before(*filter.StartTime) {
		return fmt.Errorf("end_time before start_time: %w", resource.ErrValidation)
	}
	return nil
}

func filterMatchesEvent(filter EventFilter, event Event) bool {
	if actorID := strings.TrimSpace(filter.ActorID); actorID != "" && event.ActorID != actorID {
		return false
	}
	if action := strings.TrimSpace(filter.Action); action != "" && event.Action != action {
		return false
	}
	if resourceType := strings.TrimSpace(filter.ResourceType); resourceType != "" && event.ResourceType != resourceType {
		return false
	}
	if resourceID := strings.TrimSpace(filter.ResourceID); resourceID != "" && event.ResourceID != resourceID {
		return false
	}
	if filter.StartTime != nil && event.OccurredAt.Before(filter.StartTime.UTC()) {
		return false
	}
	if filter.EndTime != nil && event.OccurredAt.After(filter.EndTime.UTC()) {
		return false
	}
	return true
}

func compareEvents(a Event, b Event) int {
	if a.OccurredAt.Equal(b.OccurredAt) {
		return strings.Compare(a.ID, b.ID)
	}
	if a.OccurredAt.After(b.OccurredAt) {
		return -1
	}
	return 1
}

func validateEvidenceInput(input EvidenceInput) error {
	if strings.TrimSpace(input.ResourceType) == "" {
		return fmt.Errorf("resource_type required: %w", resource.ErrValidation)
	}
	if strings.TrimSpace(input.ResourceID) == "" {
		return fmt.Errorf("resource_id required: %w", resource.ErrValidation)
	}
	if strings.TrimSpace(input.EvidenceType) == "" {
		return fmt.Errorf("evidence_type required: %w", resource.ErrValidation)
	}
	if strings.TrimSpace(input.StorageRef) == "" {
		return fmt.Errorf("storage_ref required: %w", resource.ErrValidation)
	}
	if strings.TrimSpace(input.DigestSHA256) == "" {
		return fmt.Errorf("digest_sha256 required: %w", resource.ErrValidation)
	}
	return nil
}

// normalizedWebhookInput is the validated/canonical webhook write model.
type normalizedWebhookInput struct {
	Name       string
	URL        string
	Secret     string
	EventTypes []string
	Status     string
}

func normalizeWebhookInput(input WebhookUpsertInput, secretRequired bool) (normalizedWebhookInput, error) {
	name := strings.TrimSpace(input.Name)
	if name == "" {
		return normalizedWebhookInput{}, fmt.Errorf("name required: %w", resource.ErrValidation)
	}

	rawURL := strings.TrimSpace(input.URL)
	if rawURL == "" {
		return normalizedWebhookInput{}, fmt.Errorf("url required: %w", resource.ErrValidation)
	}
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Host == "" {
		return normalizedWebhookInput{}, fmt.Errorf("invalid url: %w", resource.ErrValidation)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return normalizedWebhookInput{}, fmt.Errorf("unsupported url scheme: %w", resource.ErrValidation)
	}

	secret := strings.TrimSpace(input.Secret)
	if secretRequired && secret == "" {
		return normalizedWebhookInput{}, fmt.Errorf("secret required: %w", resource.ErrValidation)
	}

	eventTypes, err := normalizeEventTypes(input.EventTypes)
	if err != nil {
		return normalizedWebhookInput{}, err
	}

	// Enablement is normalized into the stored status field so downstream logic
	// only deals with one lifecycle representation.
	status := WebhookStatusDisabled
	if input.Enabled {
		status = WebhookStatusActive
	}

	return normalizedWebhookInput{
		Name:       name,
		URL:        parsed.String(),
		Secret:     secret,
		EventTypes: eventTypes,
		Status:     status,
	}, nil
}

func normalizeEventTypes(items []string) ([]string, error) {
	if len(items) == 0 {
		return nil, fmt.Errorf("event_types required: %w", resource.ErrValidation)
	}

	unique := make(map[string]struct{}, len(items))
	result := make([]string, 0, len(items))
	for _, item := range items {
		eventType := strings.TrimSpace(item)
		if eventType == "" {
			return nil, fmt.Errorf("event_types contains empty entry: %w", resource.ErrValidation)
		}
		if _, ok := unique[eventType]; ok {
			continue
		}
		unique[eventType] = struct{}{}
		result = append(result, eventType)
	}
	slices.Sort(result)
	return result, nil
}

func matchesEventType(allowed []string, eventType string) bool {
	for _, candidate := range allowed {
		switch {
		case candidate == "*":
			return true
		case strings.HasSuffix(candidate, "*"):
			// Prefix wildcards allow coarse routing such as settings.* or domain.*.
			prefix := strings.TrimSuffix(candidate, "*")
			if strings.HasPrefix(eventType, prefix) {
				return true
			}
		case candidate == eventType:
			return true
		}
	}
	return false
}

func validateExportInput(input ExportInput) error {
	switch strings.TrimSpace(input.ExportType) {
	case ExportTypeAudit, ExportTypeEvidence, ExportTypeReport:
	default:
		return fmt.Errorf("unsupported export_type %q: %w", input.ExportType, resource.ErrValidation)
	}

	switch strings.TrimSpace(input.Format) {
	case ExportFormatCSV, ExportFormatPDF, ExportFormatJSON:
	default:
		return fmt.Errorf("unsupported format %q: %w", input.Format, resource.ErrValidation)
	}

	switch strings.TrimSpace(input.Status) {
	case ExportStatusPending, ExportStatusRunning, ExportStatusSucceeded, ExportStatusFailed, ExportStatusExpired:
	default:
		return fmt.Errorf("unsupported status %q: %w", input.Status, resource.ErrValidation)
	}
	return nil
}

func cloneEvent(item Event) Event {
	item.Detail = cloneMap(item.Detail)
	return item
}

func cloneWebhookEndpoint(item WebhookEndpoint) WebhookEndpoint {
	item.EventTypes = append([]string(nil), item.EventTypes...)
	return item
}

func cloneNotification(item NotificationEvent) NotificationEvent {
	item.Payload = cloneMap(item.Payload)
	return item
}

func cloneExportRecord(item ExportRecord) ExportRecord {
	item.Filters = cloneMap(item.Filters)
	return item
}

func cloneMap(input map[string]string) map[string]string {
	if len(input) == 0 {
		return nil
	}
	cloned := make(map[string]string, len(input))
	for key, value := range input {
		cloned[key] = value
	}
	return cloned
}

func digestSecret(secret string) string {
	if strings.TrimSpace(secret) == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(secret))
	return hex.EncodeToString(sum[:])
}

// stubDeliverer is the in-memory webhook deliverer used by tests and local flows.
type stubDeliverer struct{}

func (stubDeliverer) Deliver(_ context.Context, endpoint WebhookEndpoint, _ NotificationEvent) error {
	if strings.Contains(endpoint.URL, "fail") {
		return fmt.Errorf("stub delivery failure for %s", endpoint.URL)
	}
	return nil
}
