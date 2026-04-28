package workflow

import (
	"context"

	jobscmd "github.com/zaneway/AutoCertX/internal/application/command/jobs"
	"github.com/zaneway/AutoCertX/internal/domain/job"
	"github.com/zaneway/AutoCertX/internal/scheduler"
)

// Executor adapts the issuance workflow service onto the generic scheduler worker.
type Executor struct {
	Service *Service
}

// Execute runs one claimed workflow job.
func (e Executor) Execute(ctx context.Context, claim jobscmd.ClaimedJob) scheduler.ExecutionResult {
	if e.Service == nil {
		return scheduler.ExecutionResult{
			ResultStatus: job.AttemptStatusFailed,
			ErrorCode:    "WORKFLOW_SERVICE_REQUIRED",
			ErrorMessage: "workflow service required",
		}
	}

	result := e.Service.ProcessJob(ctx, claim.Job)
	if result.ErrorCode != "" && !result.Retryable {
		return scheduler.ExecutionResult{
			ResultStatus: job.AttemptStatusFailed,
			ErrorCode:    result.ErrorCode,
			ErrorMessage: result.ErrorMessage,
		}
	}

	resultStatus := job.AttemptStatusSucceeded
	if result.Retryable {
		// T05 only schedules retries for non-succeeded job states, so transient
		// workflow conditions must surface as failed attempts with retry intent.
		resultStatus = job.AttemptStatusFailed
	}

	return scheduler.ExecutionResult{
		ResultStatus: resultStatus,
		Retryable:    result.Retryable,
		RetryDelay:   result.RetryDelay,
		ErrorCode:    result.ErrorCode,
		ErrorMessage: result.ErrorMessage,
	}
}
