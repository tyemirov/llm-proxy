package proxy

import (
	"os"
	"time"
)

// The current journal worker enters this transition under the database lock.
// Replays use the journal and never reset or recreate dispatched execution.
func (store *structuredRequestStore) beginAdmitted(requestTenant tenant, key string, admission managedJournalRequestRecord) (structuredRequestRecord, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	tenantDigest, keyDigest := sha256Hex(requestTenant.identifier.string()), sha256Hex(key)
	path, err := store.recordPath(tenantDigest, keyDigest, true)
	if err != nil {
		return structuredRequestRecord{}, err
	}
	if record, err := store.read(path); err == nil {
		if record.ProxyRequestID != admission.ExecutionID || record.IntentSHA256 != admission.IntentDigest || admission.State != journalRequestAccepted {
			return record, errUsageJournalConflict
		}
		if record.State != structuredRequestStateDispatched && record.State != structuredRequestStateNotDispatched && record.State != structuredRequestStateUncertain {
			return record, errUsageJournalConflict
		}
	} else if !os.IsNotExist(err) {
		return structuredRequestRecord{}, err
	}
	record := structuredRequestRecord{Schema: structuredRequestRecordSchema, TenantSHA256: tenantDigest, IdempotencySHA256: keyDigest,
		IntentSHA256: admission.IntentDigest, Provider: admission.Provider, Model: admission.Model, ProxyRequestID: admission.ExecutionID,
		State: structuredRequestStateDispatched, StartedAt: admission.CreatedAt.Format(time.RFC3339Nano), UpdatedAt: admission.UpdatedAt.Format(time.RFC3339Nano)}
	return record, store.publish(path, record)
}

func (store *structuredRequestStore) lookupHosted(admission managedJournalRequestRecord) (structuredRequestRecord, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	path, err := store.recordPath(sha256Hex(admission.TenantID), admission.KeyDigest, false)
	if err != nil {
		return structuredRequestRecord{}, err
	}
	record, err := store.read(path)
	if os.IsNotExist(err) {
		return structuredRequestRecord{}, errStructuredRequestNotFound
	}
	if err != nil {
		return structuredRequestRecord{}, err
	}
	if err := matchHostedResult(admission, record); err != nil {
		return structuredRequestRecord{}, err
	}
	// A published file without its database receipt is still recovery evidence.
	if admission.ResultPublishedAt != nil {
		expired, err := store.expireTerminal(path, record)
		if err != nil {
			return structuredRequestRecord{}, err
		}
		if expired {
			return structuredRequestRecord{}, errStructuredRequestNotFound
		}
	}
	return record, nil
}
