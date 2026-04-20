package scheduler

import (
	"context"
	"time"

	jobcommand "github.com/zaneway/AutoCertX/internal/application/command/jobs"
)

// ReaperService captures expired lease recovery.
type ReaperService interface {
	ReapExpired(context.Context, int, time.Time) ([]jobcommand.ReapedLease, error)
}

// Reaper recovers jobs whose worker lease has expired.
type Reaper struct {
	Service ReaperService
	Limit   int
	Clock   func() time.Time
}

// RunOnce scans and recovers expired leases.
func (r Reaper) RunOnce(ctx context.Context) (int, error) {
	reaped, err := r.Service.ReapExpired(ctx, r.Limit, r.now())
	if err != nil {
		return 0, err
	}

	return len(reaped), nil
}

func (r Reaper) now() time.Time {
	if r.Clock == nil {
		return time.Now().UTC()
	}

	return r.Clock().UTC()
}
