package scheduler

import (
	"context"
	"time"

	jobcommand "github.com/zaneway/AutoCertX/internal/application/command/jobs"
)

// PlannerService captures the baseline plan creation use case.
type PlannerService interface {
	EnsurePlans(context.Context, []jobcommand.PlanDefinition, time.Time) ([]jobcommand.PlannedJob, error)
}

// Planner creates recurring baseline jobs such as renewal scan and discovery scan.
type Planner struct {
	Service PlannerService
	Plans   []jobcommand.PlanDefinition
	Clock   func() time.Time
}

// RunOnce ensures the current plan slot exists.
func (p Planner) RunOnce(ctx context.Context) (int, error) {
	results, err := p.Service.EnsurePlans(ctx, p.Plans, p.now())
	if err != nil {
		return 0, err
	}

	created := 0
	for _, result := range results {
		if result.Created {
			created++
		}
	}

	return created, nil
}

func (p Planner) now() time.Time {
	if p.Clock == nil {
		return time.Now().UTC()
	}

	return p.Clock().UTC()
}
