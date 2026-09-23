package proxy

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/tyemirov/llm-proxy/pkg/llmproxycontract"
	"gorm.io/gorm"
)

// The journal decides whether an identity is hosted. An ordinary structured
// request continues to use its own store when no hosted identity exists.
func (service *hostedTextRequests) writeStatus(ctx *gin.Context, tenant tenant, key string) bool {
	var request managedJournalRequestRecord
	err := service.database.database.WithContext(ctx.Request.Context()).
		Where("tenant_id = ? AND key_digest = ? AND execution_kind IN ?", tenant.identifier.string(), sha256Hex(key), []journalExecutionKind{journalExecutionText, journalExecutionDictation}).
		Where("billing_account_id IN (?)", service.database.database.Model(&managedBillingAccountRecord{}).Select("id").Where("owner_user_id = ?", tenant.userID)).First(&request).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return false
	}
	if err != nil {
		writeStructuredRequestError(ctx, http.StatusInternalServerError, llmproxycontract.ErrorCodeStructuredRequestStore, "", "", "")
		return true
	}
	switch request.State {
	case journalRequestCompleted:
		record, err := service.responses.lookupHosted(request)
		if errors.Is(err, errStructuredRequestNotFound) {
			if request.ResultPublishedAt == nil {
				writeStructuredRequestRecord(ctx, hostedRequestStatusRecord(request, structuredRequestStateDispatched), service.now().UTC())
			} else {
				writeStructuredRequestError(ctx, http.StatusGone, errHostedResultExpired.Error(), structuredRequestStateSucceeded, "", request.ExecutionID)
			}
		} else if err != nil || record.ProxyRequestID != request.ExecutionID || record.IntentSHA256 != request.IntentDigest {
			writeStructuredRequestError(ctx, http.StatusInternalServerError, llmproxycontract.ErrorCodeStructuredRequestStore, "", "", request.ExecutionID)
		} else {
			writeStructuredRequestRecord(ctx, record, service.now().UTC())
		}
	case journalRequestUncertain:
		writeStructuredRequestRecord(ctx, hostedRequestStatusRecord(request, structuredRequestStateUncertain), service.now().UTC())
	case journalRequestFailed:
		writeStructuredRequestRecord(ctx, hostedRequestStatusRecord(request, structuredRequestStateFailed), service.now().UTC())
	default:
		writeStructuredRequestRecord(ctx, hostedRequestStatusRecord(request, structuredRequestStateDispatched), service.now().UTC())
	}
	return true
}
