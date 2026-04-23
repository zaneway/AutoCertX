package wiring

import (
	"context"
	"strings"

	domainscmd "github.com/zaneway/AutoCertX/internal/application/command/domains"
	auditdomain "github.com/zaneway/AutoCertX/internal/domain/audit"
)

// domainsAuditRecorder adapts domain governance audit events into the T06 audit service.
type domainsAuditRecorder struct {
	audit *auditdomain.Service
}

func (r domainsAuditRecorder) Record(ctx context.Context, event domainscmd.AuditEvent) {
	if r.audit == nil {
		return
	}

	actorID := strings.TrimSpace(event.ActorID)
	actorType := auditdomain.ActorTypeSystem
	if actorID == "" {
		// Governance events without an actor are recorded as system activity so
		// downstream audit queries can still distinguish them from user actions.
		actorID = "system"
	} else {
		actorType = auditdomain.ActorTypeUser
	}

	_, _ = r.audit.RecordEvent(ctx, event.Scope, auditdomain.EventInput{
		ActorType:    actorType,
		ActorID:      actorID,
		Action:       event.Action,
		ResourceType: event.ResourceType,
		ResourceID:   event.ResourceID,
		Detail:       event.Metadata,
		OccurredAt:   event.OccurredAt,
	})
}
