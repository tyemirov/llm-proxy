package proxy

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

const (
	structuredRequestRecordSchema       = "llm-proxy.structured-request.v1"
	structuredRequestStateNotDispatched = "not_dispatched"
	structuredRequestStateDispatched    = "dispatched"
	structuredRequestStateSucceeded     = "succeeded"
	structuredRequestStateFailed        = "failed"
	structuredRequestStateUncertain     = "uncertain"
	structuredRequestDirectoryName      = "structured-requests"
)

var (
	errStructuredRequestConflict = errors.New("structured request idempotency conflict")
	errStructuredRequestNotFound = errors.New("structured request not found")
	idempotencyKeyPattern        = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$`)
	digestPattern                = regexp.MustCompile(`^[0-9a-f]{64}$`)
	structuredRequestTempPattern = regexp.MustCompile(`^\.structured-request-[0-9]+\.tmp$`)
	structuredRequestLstat       = os.Lstat
	structuredRequestChmod       = os.Chmod
	structuredRequestMkdirAll    = os.MkdirAll
	structuredRequestWalkDir     = filepath.WalkDir
	structuredRequestRemove      = os.Remove
	structuredRequestReadFile    = os.ReadFile
	structuredRequestMarshal     = json.MarshalIndent
	structuredRequestCreateTemp  = os.CreateTemp
	structuredRequestFileChmod   = func(file *os.File, mode fs.FileMode) error { return file.Chmod(mode) }
	structuredRequestFileWrite   = func(file *os.File, data []byte) (int, error) { return file.Write(data) }
	structuredRequestFileSync    = func(file *os.File) error { return file.Sync() }
	structuredRequestFileClose   = func(file *os.File) error { return file.Close() }
	structuredRequestRename      = os.Rename
	structuredRequestOpen        = os.Open
)

type structuredRequestRecord struct {
	Schema            string          `json:"schema"`
	TenantSHA256      string          `json:"tenant_sha256"`
	IdempotencySHA256 string          `json:"idempotency_sha256"`
	IntentSHA256      string          `json:"intent_sha256"`
	Provider          string          `json:"provider"`
	Model             string          `json:"model"`
	ProxyRequestID    string          `json:"proxy_request_id"`
	State             string          `json:"state"`
	StartedAt         string          `json:"started_at"`
	UpdatedAt         string          `json:"updated_at"`
	CompletedAt       string          `json:"completed_at,omitempty"`
	StatusCode        int             `json:"status_code,omitempty"`
	FailureCode       string          `json:"failure_code,omitempty"`
	Result            json.RawMessage `json:"result,omitempty"`
}

type structuredRequestStore struct {
	mu        sync.Mutex
	root      string
	retention time.Duration
	now       func() time.Time
	publish   func(string, structuredRequestRecord) error
	read      func(string) (structuredRequestRecord, error)
}

func newStructuredRequestStore(assetStorePath string, retentionSeconds int) (*structuredRequestStore, error) {
	root := filepath.Join(assetStorePath, structuredRequestDirectoryName)
	store := &structuredRequestStore{
		root: root, retention: time.Duration(retentionSeconds) * time.Second,
		now:     func() time.Time { return time.Now().UTC() },
		publish: publishStructuredRequestRecord,
		read:    readStructuredRequestRecord,
	}
	if _, statError := structuredRequestLstat(root); os.IsNotExist(statError) {
		return store, nil
	} else if statError != nil {
		return nil, fmt.Errorf("inspect structured request store: %w", statError)
	}
	if directoryError := ensurePrivateDirectory(root); directoryError != nil {
		return nil, fmt.Errorf("initialize structured request store: %w", directoryError)
	}
	if recoveryError := store.recoverInterrupted(); recoveryError != nil {
		return nil, recoveryError
	}
	return store, nil
}

func ensurePrivateDirectory(path string) error {
	if info, statError := structuredRequestLstat(path); statError == nil {
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return fmt.Errorf("path is not a direct directory: %s", path)
		}
		return structuredRequestChmod(path, 0o700)
	} else if !os.IsNotExist(statError) {
		return statError
	}
	if makeError := structuredRequestMkdirAll(path, 0o700); makeError != nil {
		return makeError
	}
	return structuredRequestChmod(path, 0o700)
}

func (store *structuredRequestStore) recoverInterrupted() error {
	store.mu.Lock()
	defer store.mu.Unlock()
	now := store.now().UTC()
	return structuredRequestWalkDir(store.root, func(path string, entry fs.DirEntry, walkError error) error {
		if walkError != nil {
			return walkError
		}
		if path == store.root {
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("structured request store contains symlink: %s", path)
		}
		if entry.IsDir() {
			return nil
		}
		if structuredRequestTemporaryFile(store.root, path, entry) {
			return structuredRequestRemove(path)
		}
		if filepath.Ext(path) != ".json" {
			return fmt.Errorf("structured request store contains unexpected file: %s", path)
		}
		record, readError := store.read(path)
		if readError != nil {
			return readError
		}
		updatedAt, timeError := time.Parse(time.RFC3339Nano, record.UpdatedAt)
		if timeError != nil {
			return fmt.Errorf("decode structured request updated_at: %w", timeError)
		}
		if record.State == structuredRequestStateUncertain {
			return nil
		}
		if record.State == structuredRequestStateDispatched {
			record.State = structuredRequestStateUncertain
			record.UpdatedAt = now.Format(time.RFC3339Nano)
			record.CompletedAt = record.UpdatedAt
			record.StatusCode = http.StatusConflict
			record.FailureCode = "structured_request_interrupted"
			return store.publish(path, record)
		}
		if store.retention > 0 && now.Sub(updatedAt) >= store.retention {
			return structuredRequestRemove(path)
		}
		return nil
	})
}

func structuredRequestTemporaryFile(root string, path string, entry fs.DirEntry) bool {
	tenantDirectory := filepath.Dir(path)
	return entry.Type().IsRegular() &&
		structuredRequestTempPattern.MatchString(entry.Name()) &&
		filepath.Dir(tenantDirectory) == root &&
		digestPattern.MatchString(filepath.Base(tenantDirectory))
}

func validIdempotencyKey(rawKey string) bool {
	return idempotencyKeyPattern.MatchString(rawKey)
}

func structuredRequestIntent(provider string, model string, canonicalBody []byte) string {
	hash := sha256.New()
	_, _ = io.WriteString(hash, provider)
	_, _ = io.WriteString(hash, "\n")
	_, _ = io.WriteString(hash, model)
	_, _ = io.WriteString(hash, "\n")
	_, _ = hash.Write(canonicalBody)
	return hex.EncodeToString(hash.Sum(nil))
}

func canonicalJSON(raw []byte) ([]byte, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var value any
	if decodeError := decoder.Decode(&value); decodeError != nil {
		return nil, decodeError
	}
	if decodeError := decoder.Decode(&struct{}{}); decodeError != io.EOF {
		return nil, errors.New("JSON body must contain one value")
	}
	return json.Marshal(value)
}

func (store *structuredRequestStore) begin(requestTenant tenant, idempotencyKey string, intentSHA256 string, provider string, model string, proxyRequestID string) (structuredRequestRecord, bool, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	tenantDigest := sha256Hex(requestTenant.identifier.string())
	keyDigest := sha256Hex(idempotencyKey)
	path, pathError := store.recordPath(tenantDigest, keyDigest, true)
	if pathError != nil {
		return structuredRequestRecord{}, false, pathError
	}
	record, readError := store.read(path)
	if readError == nil {
		expired, expirationError := store.expireTerminal(path, record)
		if expirationError != nil {
			return structuredRequestRecord{}, false, expirationError
		}
		if expired {
			readError = os.ErrNotExist
		} else {
			if record.IntentSHA256 != intentSHA256 {
				return structuredRequestRecord{}, false, errStructuredRequestConflict
			}
			if record.State == structuredRequestStateFailed {
				now := store.now().UTC().Format(time.RFC3339Nano)
				record.ProxyRequestID = proxyRequestID
				record.State = structuredRequestStateNotDispatched
				record.StartedAt = now
				record.UpdatedAt = now
				record.CompletedAt = ""
				record.StatusCode = 0
				record.FailureCode = ""
				record.Result = nil
				if publishError := store.publish(path, record); publishError != nil {
					return structuredRequestRecord{}, false, publishError
				}
				return record, true, nil
			}
			return record, false, nil
		}
	}
	if !os.IsNotExist(readError) {
		return structuredRequestRecord{}, false, readError
	}
	now := store.now().UTC().Format(time.RFC3339Nano)
	record = structuredRequestRecord{
		Schema: structuredRequestRecordSchema, TenantSHA256: tenantDigest,
		IdempotencySHA256: keyDigest, IntentSHA256: intentSHA256,
		Provider: provider, Model: model, ProxyRequestID: proxyRequestID,
		State: structuredRequestStateNotDispatched, StartedAt: now, UpdatedAt: now,
	}
	if publishError := store.publish(path, record); publishError != nil {
		return structuredRequestRecord{}, false, publishError
	}
	return record, true, nil
}

func (store *structuredRequestStore) lookup(requestTenant tenant, idempotencyKey string) (structuredRequestRecord, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	tenantDigest := sha256Hex(requestTenant.identifier.string())
	path, _ := store.recordPath(tenantDigest, sha256Hex(idempotencyKey), false)
	record, readError := store.read(path)
	if os.IsNotExist(readError) {
		return structuredRequestRecord{}, errStructuredRequestNotFound
	}
	if readError == nil {
		expired, expirationError := store.expireTerminal(path, record)
		if expirationError != nil {
			return structuredRequestRecord{}, expirationError
		}
		if expired {
			return structuredRequestRecord{}, errStructuredRequestNotFound
		}
	}
	return record, readError
}

func (store *structuredRequestStore) expireTerminal(path string, record structuredRequestRecord) (bool, error) {
	if store.retention <= 0 || record.State == structuredRequestStateUncertain ||
		(record.State != structuredRequestStateSucceeded && record.State != structuredRequestStateFailed) {
		return false, nil
	}
	updatedAt, timeError := time.Parse(time.RFC3339Nano, record.UpdatedAt)
	if timeError != nil {
		return false, fmt.Errorf("decode structured request updated_at: %w", timeError)
	}
	if store.now().UTC().Sub(updatedAt) < store.retention {
		return false, nil
	}
	if removeError := structuredRequestRemove(path); removeError != nil {
		return false, fmt.Errorf("expire structured request record: %w", removeError)
	}
	return true, nil
}

func (store *structuredRequestStore) transition(requestTenant tenant, idempotencyKey string, intentSHA256 string, update func(*structuredRequestRecord, time.Time)) (structuredRequestRecord, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	tenantDigest := sha256Hex(requestTenant.identifier.string())
	path, _ := store.recordPath(tenantDigest, sha256Hex(idempotencyKey), false)
	record, readError := store.read(path)
	if readError != nil {
		return structuredRequestRecord{}, readError
	}
	if record.IntentSHA256 != intentSHA256 {
		return structuredRequestRecord{}, errStructuredRequestConflict
	}
	now := store.now().UTC()
	update(&record, now)
	record.UpdatedAt = now.Format(time.RFC3339Nano)
	if publishError := store.publish(path, record); publishError != nil {
		return structuredRequestRecord{}, publishError
	}
	return record, nil
}

func (store *structuredRequestStore) claimDispatch(requestTenant tenant, idempotencyKey string, intentSHA256 string) (bool, error) {
	claimed := false
	_, transitionError := store.transition(requestTenant, idempotencyKey, intentSHA256, func(record *structuredRequestRecord, _ time.Time) {
		if record.State != structuredRequestStateNotDispatched {
			return
		}
		record.State = structuredRequestStateDispatched
		claimed = true
	})
	return claimed, transitionError
}

func (store *structuredRequestStore) succeed(requestTenant tenant, idempotencyKey string, intentSHA256 string, result string) error {
	canonicalResult, canonicalError := canonicalJSON([]byte(result))
	if canonicalError != nil {
		return canonicalError
	}
	_, transitionError := store.transition(requestTenant, idempotencyKey, intentSHA256, func(record *structuredRequestRecord, now time.Time) {
		record.State = structuredRequestStateSucceeded
		record.CompletedAt = now.Format(time.RFC3339Nano)
		record.StatusCode = 200
		record.Result = append(json.RawMessage(nil), canonicalResult...)
		record.FailureCode = ""
	})
	return transitionError
}

func (store *structuredRequestStore) fail(requestTenant tenant, idempotencyKey string, intentSHA256 string, statusCode int, failureCode string) error {
	_, transitionError := store.transition(requestTenant, idempotencyKey, intentSHA256, func(record *structuredRequestRecord, now time.Time) {
		record.State = structuredRequestStateFailed
		record.CompletedAt = now.Format(time.RFC3339Nano)
		record.StatusCode = statusCode
		record.FailureCode = failureCode
		record.Result = nil
	})
	return transitionError
}

func (store *structuredRequestStore) uncertain(requestTenant tenant, idempotencyKey string, intentSHA256 string) error {
	_, transitionError := store.transition(requestTenant, idempotencyKey, intentSHA256, func(record *structuredRequestRecord, now time.Time) {
		record.State = structuredRequestStateUncertain
		record.CompletedAt = now.Format(time.RFC3339Nano)
		record.StatusCode = 409
		record.FailureCode = "structured_request_outcome_unknown"
		record.Result = nil
	})
	return transitionError
}

func (store *structuredRequestStore) recordPath(tenantDigest string, keyDigest string, createTenantDirectory bool) (string, error) {
	if !digestPattern.MatchString(tenantDigest) || !digestPattern.MatchString(keyDigest) {
		return "", errors.New("invalid structured request digest")
	}
	tenantPath := filepath.Join(store.root, tenantDigest)
	if createTenantDirectory {
		if directoryError := ensurePrivateDirectory(store.root); directoryError != nil {
			return "", directoryError
		}
		if directoryError := ensurePrivateDirectory(tenantPath); directoryError != nil {
			return "", directoryError
		}
	}
	return filepath.Join(tenantPath, keyDigest+".json"), nil
}

func sha256Hex(value string) string {
	digest := sha256.Sum256([]byte(value))
	return hex.EncodeToString(digest[:])
}

func readStructuredRequestRecord(path string) (structuredRequestRecord, error) {
	info, statError := structuredRequestLstat(path)
	if statError != nil {
		return structuredRequestRecord{}, statError
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return structuredRequestRecord{}, fmt.Errorf("structured request record is not a direct regular file: %s", path)
	}
	data, readError := structuredRequestReadFile(path)
	if readError != nil {
		return structuredRequestRecord{}, readError
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var record structuredRequestRecord
	if decodeError := decoder.Decode(&record); decodeError != nil {
		return structuredRequestRecord{}, decodeError
	}
	if decodeError := decoder.Decode(&struct{}{}); decodeError != io.EOF {
		return structuredRequestRecord{}, errors.New("structured request record must contain one JSON value")
	}
	if validationError := validateStructuredRequestRecord(record); validationError != nil {
		return structuredRequestRecord{}, validationError
	}
	return record, nil
}

func validateStructuredRequestRecord(record structuredRequestRecord) error {
	if record.Schema != structuredRequestRecordSchema || !digestPattern.MatchString(record.TenantSHA256) ||
		!digestPattern.MatchString(record.IdempotencySHA256) || !digestPattern.MatchString(record.IntentSHA256) ||
		strings.TrimSpace(record.Provider) == "" || strings.TrimSpace(record.Model) == "" ||
		strings.TrimSpace(record.ProxyRequestID) == "" || strings.TrimSpace(record.StartedAt) == "" ||
		strings.TrimSpace(record.UpdatedAt) == "" {
		return errors.New("invalid structured request record")
	}
	switch record.State {
	case structuredRequestStateNotDispatched, structuredRequestStateDispatched:
		if record.CompletedAt != "" || record.StatusCode != 0 || record.FailureCode != "" || len(record.Result) != 0 {
			return errors.New("invalid nonterminal structured request record")
		}
	case structuredRequestStateSucceeded:
		if record.StatusCode != 200 || record.CompletedAt == "" || record.FailureCode != "" || !json.Valid(record.Result) {
			return errors.New("invalid succeeded structured request record")
		}
	case structuredRequestStateFailed, structuredRequestStateUncertain:
		if record.StatusCode < 400 || record.CompletedAt == "" || record.FailureCode == "" || len(record.Result) != 0 {
			return errors.New("invalid failed structured request record")
		}
	default:
		return errors.New("invalid structured request state")
	}
	return nil
}

func publishStructuredRequestRecord(path string, record structuredRequestRecord) error {
	if validationError := validateStructuredRequestRecord(record); validationError != nil {
		return validationError
	}
	data, marshalError := structuredRequestMarshal(record, "", "  ")
	if marshalError != nil {
		return marshalError
	}
	directory := filepath.Dir(path)
	temporary, createError := structuredRequestCreateTemp(directory, ".structured-request-*.tmp")
	if createError != nil {
		return createError
	}
	temporaryPath := temporary.Name()
	committed := false
	defer func() {
		_ = structuredRequestFileClose(temporary)
		if !committed {
			_ = structuredRequestRemove(temporaryPath)
		}
	}()
	if chmodError := structuredRequestFileChmod(temporary, 0o600); chmodError != nil {
		return chmodError
	}
	if _, writeError := structuredRequestFileWrite(temporary, append(data, '\n')); writeError != nil {
		return writeError
	}
	if syncError := structuredRequestFileSync(temporary); syncError != nil {
		return syncError
	}
	if closeError := structuredRequestFileClose(temporary); closeError != nil {
		return closeError
	}
	if renameError := structuredRequestRename(temporaryPath, path); renameError != nil {
		return renameError
	}
	committed = true
	directoryHandle, openError := structuredRequestOpen(directory)
	if openError != nil {
		return openError
	}
	syncError := structuredRequestFileSync(directoryHandle)
	closeError := structuredRequestFileClose(directoryHandle)
	return errors.Join(syncError, closeError)
}
