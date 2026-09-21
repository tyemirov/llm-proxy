package proxy

import (
	"context"

	"github.com/tyemirov/llm-proxy/internal/constants"
)

type upstreamAdmissionDecision string

const (
	logEventUpstreamAdmission                                 = "upstream HTTP admission"
	logFieldAdmissionDecision                                 = "decision"
	logFieldAdmissionClass                                    = "work_class"
	logFieldAdmissionGlobalActive                             = "global_active"
	logFieldAdmissionGlobalAdmitted                           = "global_admitted"
	logFieldAdmissionOriginActive                             = "origin_active"
	logFieldAdmissionOriginQueued                             = "origin_queued"
	logFieldAdmissionOperationID                              = "operation_id"
	upstreamDecisionAdmitted        upstreamAdmissionDecision = "admitted"
	upstreamDecisionRejected        upstreamAdmissionDecision = "rejected"
	upstreamDecisionActive          upstreamAdmissionDecision = "active"
	upstreamDecisionCapacityWait    upstreamAdmissionDecision = "capacity_wait"
	upstreamDecisionRateWait        upstreamAdmissionDecision = "rate_wait"
	upstreamDecisionCanceled        upstreamAdmissionDecision = "canceled"
	upstreamDecisionReleased        upstreamAdmissionDecision = "released"
)

var upstreamWorkClassNames = [upstreamWorkClassCount]string{"interactive", "media", "status", "transfer"}

// Admission events contain origin, bounded counters, and opaque correlation IDs.
// Request bodies, credentials, account fields, and provider errors are excluded.
func (doer *admissionHTTPDoer) logAdmission(ctx context.Context, originName string, scope upstreamRequestScope, decision upstreamAdmissionDecision) {
	scheduler := doer.scheduler
	scheduler.mutex.Lock()
	global := scheduler.usage
	origin := upstreamCapacityUsage{}
	if declared := scheduler.origins[originName]; declared != nil {
		origin = declared.usage
	}
	scheduler.mutex.Unlock()
	fields := []any{
		constants.LogFieldUpstreamOrigin, originName,
		logFieldAdmissionDecision, string(decision),
		logFieldAdmissionClass, upstreamWorkClassNames[scope.class],
		logFieldAdmissionGlobalActive, global.active,
		logFieldAdmissionGlobalAdmitted, global.admitted,
		logFieldAdmissionOriginActive, origin.active,
		logFieldAdmissionOriginQueued, origin.admitted - origin.active,
	}
	if requestID, present := ctx.Value(requestIdentifierContextKey{}).(string); present {
		fields = append(fields, logFieldRequestID, requestID)
	}
	if scope.operationID != "" {
		fields = append(fields, logFieldAdmissionOperationID, scope.operationID)
	}
	doer.logger.Infow(logEventUpstreamAdmission, fields...)
}
