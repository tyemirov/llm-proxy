package proxy

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/tyemirov/llm-proxy/internal/constants"
	"github.com/tyemirov/llm-proxy/pkg/llmproxycontract"
)

const (
	assetMetadataVersion = 2
	assetStateAvailable  = "available"
	assetStateDeleted    = "deleted"
	assetFileMode        = 0o600
	assetDirectoryMode   = 0o700
)

var (
	errAssetInvalid      = errors.New("asset_invalid")
	errAssetNotFound     = errors.New("asset_not_found")
	errAssetExpired      = errors.New("asset_expired")
	errAssetDeleted      = errors.New("asset_deleted")
	errAssetMIMEMismatch = errors.New("asset_mime_mismatch")
	errAssetTooLarge     = errors.New("asset_too_large")
	errAssetStore        = errors.New("asset_store_error")

	assetIdentifierPattern = regexp.MustCompile(`^ast_[0-9a-f]{32}$`)
)

type tenantAssetMetadata struct {
	Version       int        `json:"version"`
	AssetID       string     `json:"asset_id"`
	TenantID      string     `json:"tenant_id"`
	MIMEType      string     `json:"mime_type"`
	SizeBytes     int64      `json:"size_bytes"`
	ContentSHA256 string     `json:"content_sha256"`
	State         string     `json:"state"`
	CreatedAt     time.Time  `json:"created_at"`
	ExpiresAt     time.Time  `json:"expires_at"`
	DeletedAt     *time.Time `json:"deleted_at,omitempty"`
}

type tenantAssetResponse struct {
	AssetID   string    `json:"asset_id"`
	MIMEType  string    `json:"mime_type"`
	SizeBytes int64     `json:"size_bytes"`
	State     string    `json:"state"`
	CreatedAt time.Time `json:"created_at"`
	ExpiresAt time.Time `json:"expires_at"`
}

type tenantAssetErrorEnvelope struct {
	Error tenantAssetErrorDetail `json:"error"`
}

type tenantAssetErrorDetail struct {
	Code string `json:"code"`
}

type tenantAssetReader struct {
	metadata tenantAssetMetadata
	file     *os.File
}

func (reader *tenantAssetReader) Close() error {
	return reader.file.Close()
}

type tenantAssetStore struct {
	root          string
	maxAssetBytes int64
	retention     time.Duration
	now           func() time.Time
	mutex         sync.Mutex
	initialized   bool
	cleanupError  error
}

var (
	assetMkdirAll   = os.MkdirAll
	assetChmod      = os.Chmod
	assetCreateTemp = os.CreateTemp
	assetCopy       = io.Copy
	assetClose      = func(file *os.File) error { return file.Close() }
	assetRename     = os.Rename
	assetRemove     = os.Remove
	assetOpen       = os.Open
	assetReadDir    = os.ReadDir
	assetStat       = func(file *os.File) (os.FileInfo, error) { return file.Stat() }
	assetSeek       = func(file *os.File, offset int64, whence int) (int64, error) { return file.Seek(offset, whence) }
	assetWrite      = func(file *os.File, data []byte) (int, error) { return file.Write(data) }
	assetAfterFunc  = time.AfterFunc
)

func newTenantAssetStore(root string, maxAssetBytes int64, retentionSeconds int) *tenantAssetStore {
	return &tenantAssetStore{
		root:          root,
		maxAssetBytes: maxAssetBytes,
		retention:     time.Duration(retentionSeconds) * time.Second,
		now:           time.Now,
	}
}

func (store *tenantAssetStore) upload(requestTenant tenant, mimeType string, source io.Reader) (tenantAssetMetadata, error) {
	if !supportedMessageMediaMIME(mimeType) {
		return tenantAssetMetadata{}, errAssetInvalid
	}
	store.mutex.Lock()
	initializationError := store.initializeLocked()
	store.mutex.Unlock()
	if initializationError != nil {
		return tenantAssetMetadata{}, initializationError
	}
	temporaryFile, temporaryError := assetCreateTemp(store.root, ".asset-upload-*")
	if temporaryError != nil {
		return tenantAssetMetadata{}, fmt.Errorf("%w: create temporary file", errAssetStore)
	}
	temporaryPath := temporaryFile.Name()
	defer assetRemove(temporaryPath)
	if chmodError := assetChmod(temporaryPath, assetFileMode); chmodError != nil {
		assetClose(temporaryFile)
		return tenantAssetMetadata{}, fmt.Errorf("%w: set file mode", errAssetStore)
	}
	hasher := sha256.New()
	written, copyError := assetCopy(io.MultiWriter(temporaryFile, hasher), io.LimitReader(source, store.maxAssetBytes+1))
	closeError := assetClose(temporaryFile)
	if copyError != nil || closeError != nil {
		return tenantAssetMetadata{}, fmt.Errorf("%w: write asset", errAssetStore)
	}
	if written == 0 {
		return tenantAssetMetadata{}, errAssetInvalid
	}
	if written > store.maxAssetBytes {
		return tenantAssetMetadata{}, errAssetTooLarge
	}
	store.mutex.Lock()
	defer store.mutex.Unlock()
	if store.cleanupError != nil {
		return tenantAssetMetadata{}, store.cleanupError
	}
	assetID := newAssetIdentifier()
	createdAt := store.now().UTC()
	metadata := tenantAssetMetadata{
		Version:       assetMetadataVersion,
		AssetID:       assetID,
		TenantID:      requestTenant.identifier.string(),
		MIMEType:      mimeType,
		SizeBytes:     written,
		ContentSHA256: hex.EncodeToString(hasher.Sum(nil)),
		State:         assetStateAvailable,
		CreatedAt:     createdAt,
		ExpiresAt:     createdAt.Add(store.retention),
	}
	dataPath := store.dataPath(assetID)
	if renameError := assetRename(temporaryPath, dataPath); renameError != nil {
		return tenantAssetMetadata{}, fmt.Errorf("%w: publish data", errAssetStore)
	}
	if metadataError := store.writeMetadata(metadata); metadataError != nil {
		assetRemove(dataPath)
		return tenantAssetMetadata{}, metadataError
	}
	store.scheduleExpirationLocked(metadata)
	return metadata, nil
}

func (store *tenantAssetStore) resolve(requestTenant tenant, assetID string, expectedMIMEType string) (*tenantAssetReader, error) {
	if !assetIdentifierPattern.MatchString(assetID) || !supportedMessageMediaMIME(expectedMIMEType) {
		return nil, errAssetInvalid
	}
	store.mutex.Lock()
	if initializationError := store.initializeLocked(); initializationError != nil {
		store.mutex.Unlock()
		return nil, initializationError
	}
	metadata, metadataError := store.readMetadata(assetID)
	if metadataError != nil {
		store.mutex.Unlock()
		return nil, metadataError
	}
	if metadata.TenantID != requestTenant.identifier.string() {
		store.mutex.Unlock()
		return nil, errAssetNotFound
	}
	if metadata.State == assetStateDeleted {
		store.mutex.Unlock()
		return nil, errAssetDeleted
	}
	if !store.now().UTC().Before(metadata.ExpiresAt) {
		if removeError := assetRemove(store.dataPath(assetID)); removeError != nil && !errors.Is(removeError, os.ErrNotExist) {
			store.mutex.Unlock()
			return nil, errAssetStore
		}
		store.mutex.Unlock()
		return nil, errAssetExpired
	}
	if metadata.MIMEType != expectedMIMEType {
		store.mutex.Unlock()
		return nil, errAssetMIMEMismatch
	}
	dataFile, openError := assetOpen(store.dataPath(assetID))
	if openError != nil {
		store.mutex.Unlock()
		return nil, errAssetStore
	}
	store.mutex.Unlock()
	valid := false
	defer func() {
		if !valid {
			assetClose(dataFile)
		}
	}()
	fileInfo, statError := assetStat(dataFile)
	if statError != nil || !fileInfo.Mode().IsRegular() || fileInfo.Size() != metadata.SizeBytes {
		return nil, errAssetStore
	}
	hasher := sha256.New()
	if _, hashError := assetCopy(hasher, dataFile); hashError != nil {
		return nil, errAssetStore
	}
	if hex.EncodeToString(hasher.Sum(nil)) != metadata.ContentSHA256 {
		return nil, errAssetStore
	}
	if _, seekError := assetSeek(dataFile, 0, io.SeekStart); seekError != nil {
		return nil, errAssetStore
	}
	valid = true
	return &tenantAssetReader{metadata: metadata, file: dataFile}, nil
}

func (store *tenantAssetStore) delete(requestTenant tenant, assetID string) error {
	if !assetIdentifierPattern.MatchString(assetID) {
		return errAssetNotFound
	}
	store.mutex.Lock()
	defer store.mutex.Unlock()
	if initializationError := store.initializeLocked(); initializationError != nil {
		return initializationError
	}
	metadata, metadataError := store.readMetadata(assetID)
	if metadataError != nil {
		return metadataError
	}
	if metadata.TenantID != requestTenant.identifier.string() {
		return errAssetNotFound
	}
	if metadata.State == assetStateDeleted {
		if removeError := assetRemove(store.dataPath(assetID)); removeError != nil && !errors.Is(removeError, os.ErrNotExist) {
			return errAssetStore
		}
		return errAssetDeleted
	}
	if !store.now().UTC().Before(metadata.ExpiresAt) {
		if removeError := assetRemove(store.dataPath(assetID)); removeError != nil && !errors.Is(removeError, os.ErrNotExist) {
			return errAssetStore
		}
		return errAssetExpired
	}
	deletedAt := store.now().UTC()
	metadata.State = assetStateDeleted
	metadata.DeletedAt = &deletedAt
	if metadataError := store.writeMetadata(metadata); metadataError != nil {
		return metadataError
	}
	if removeError := assetRemove(store.dataPath(assetID)); removeError != nil && !errors.Is(removeError, os.ErrNotExist) {
		return errAssetStore
	}
	return nil
}

func (store *tenantAssetStore) initializeLocked() error {
	if store.cleanupError != nil {
		return store.cleanupError
	}
	if store.initialized {
		return nil
	}
	if directoryError := assetMkdirAll(store.root, assetDirectoryMode); directoryError != nil {
		return fmt.Errorf("%w: create directory", errAssetStore)
	}
	if chmodError := assetChmod(store.root, assetDirectoryMode); chmodError != nil {
		return fmt.Errorf("%w: set directory mode", errAssetStore)
	}
	entries, readDirectoryError := assetReadDir(store.root)
	if readDirectoryError != nil {
		return fmt.Errorf("%w: read directory", errAssetStore)
	}
	availableAssets := make([]tenantAssetMetadata, 0)
	for _, entry := range entries {
		entryName := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(entryName, ".json") {
			continue
		}
		assetID := strings.TrimSuffix(entryName, ".json")
		if !assetIdentifierPattern.MatchString(assetID) {
			continue
		}
		metadata, metadataError := store.readMetadata(assetID)
		if metadataError != nil {
			return metadataError
		}
		if metadata.State == assetStateDeleted || !store.now().UTC().Before(metadata.ExpiresAt) {
			if removeError := assetRemove(store.dataPath(assetID)); removeError != nil && !errors.Is(removeError, os.ErrNotExist) {
				return fmt.Errorf("%w: reclaim asset", errAssetStore)
			}
			continue
		}
		availableAssets = append(availableAssets, metadata)
	}
	store.initialized = true
	for _, metadata := range availableAssets {
		store.scheduleExpirationLocked(metadata)
	}
	return nil
}

func (store *tenantAssetStore) scheduleExpirationLocked(metadata tenantAssetMetadata) {
	delay := metadata.ExpiresAt.Sub(store.now().UTC())
	if delay < 0 {
		delay = 0
	}
	assetAfterFunc(delay, func() {
		store.expireAsset(metadata.AssetID, metadata.ExpiresAt)
	})
}

func (store *tenantAssetStore) expireAsset(assetID string, expectedExpiry time.Time) {
	store.mutex.Lock()
	defer store.mutex.Unlock()
	if store.cleanupError != nil {
		return
	}
	metadata, metadataError := store.readMetadata(assetID)
	if errors.Is(metadataError, errAssetNotFound) {
		return
	}
	if metadataError != nil {
		store.cleanupError = metadataError
		return
	}
	if metadata.ExpiresAt != expectedExpiry || metadata.State == assetStateDeleted || store.now().UTC().Before(metadata.ExpiresAt) {
		return
	}
	if removeError := assetRemove(store.dataPath(assetID)); removeError != nil && !errors.Is(removeError, os.ErrNotExist) {
		store.cleanupError = fmt.Errorf("%w: expire asset", errAssetStore)
	}
}

func (store *tenantAssetStore) readMetadata(assetID string) (tenantAssetMetadata, error) {
	metadataFile, openError := assetOpen(store.metadataPath(assetID))
	if errors.Is(openError, os.ErrNotExist) {
		return tenantAssetMetadata{}, errAssetNotFound
	}
	if openError != nil {
		return tenantAssetMetadata{}, errAssetStore
	}
	defer assetClose(metadataFile)
	var metadata tenantAssetMetadata
	decoder := json.NewDecoder(io.LimitReader(metadataFile, 4097))
	decoder.DisallowUnknownFields()
	if decodeError := decoder.Decode(&metadata); decodeError != nil {
		return tenantAssetMetadata{}, errAssetStore
	}
	if trailingError := decoder.Decode(&struct{}{}); !errors.Is(trailingError, io.EOF) {
		return tenantAssetMetadata{}, errAssetStore
	}
	validState := metadata.State == assetStateAvailable && metadata.DeletedAt == nil
	validState = validState || (metadata.State == assetStateDeleted && metadata.DeletedAt != nil && !metadata.DeletedAt.Before(metadata.CreatedAt))
	if metadata.Version != assetMetadataVersion || metadata.AssetID != assetID || metadata.TenantID == constants.EmptyString || !supportedMessageMediaMIME(metadata.MIMEType) || metadata.SizeBytes <= 0 || !canonicalSHA256(metadata.ContentSHA256) || metadata.CreatedAt.IsZero() || !metadata.ExpiresAt.After(metadata.CreatedAt) || !validState {
		return tenantAssetMetadata{}, errAssetStore
	}
	return metadata, nil
}

func (store *tenantAssetStore) writeMetadata(metadata tenantAssetMetadata) error {
	metadataBytes, _ := json.Marshal(metadata)
	temporaryFile, temporaryError := assetCreateTemp(store.root, ".asset-metadata-*")
	if temporaryError != nil {
		return errAssetStore
	}
	temporaryPath := temporaryFile.Name()
	defer assetRemove(temporaryPath)
	if chmodError := assetChmod(temporaryPath, assetFileMode); chmodError != nil {
		assetClose(temporaryFile)
		return errAssetStore
	}
	if _, writeError := assetWrite(temporaryFile, metadataBytes); writeError != nil {
		assetClose(temporaryFile)
		return errAssetStore
	}
	if closeError := assetClose(temporaryFile); closeError != nil {
		return errAssetStore
	}
	if renameError := assetRename(temporaryPath, store.metadataPath(metadata.AssetID)); renameError != nil {
		return errAssetStore
	}
	return nil
}

func (store *tenantAssetStore) dataPath(assetID string) string {
	return filepath.Join(store.root, assetID+".data")
}

func (store *tenantAssetStore) metadataPath(assetID string) string {
	return filepath.Join(store.root, assetID+".json")
}

func newAssetIdentifier() string {
	randomBytes := make([]byte, 16)
	_, _ = rand.Read(randomBytes)
	return "ast_" + hex.EncodeToString(randomBytes)
}

func canonicalSHA256(rawDigest string) bool {
	digestBytes, digestError := hex.DecodeString(rawDigest)
	return digestError == nil && len(digestBytes) == sha256.Size && strings.ToLower(rawDigest) == rawDigest
}

func tenantAssetUploadHandler(store *tenantAssetStore) gin.HandlerFunc {
	return func(ginContext *gin.Context) {
		if ginContext.Request.ContentLength > store.maxAssetBytes {
			writeTenantAssetError(ginContext, errAssetTooLarge)
			return
		}
		if !validTenantAssetUploadHeaders(ginContext.Request) {
			writeTenantAssetError(ginContext, errAssetInvalid)
			return
		}
		metadata, uploadError := store.upload(
			authenticatedTenantFromContext(ginContext),
			strings.TrimSpace(ginContext.GetHeader(headerContentType)),
			ginContext.Request.Body,
		)
		if uploadError != nil {
			if requestContextEnded(ginContext) {
				return
			}
			writeTenantAssetError(ginContext, uploadError)
			return
		}
		markRequestOutcome(ginContext, requestOutcomeSuccess, managedUsageOutcomeSuccess)
		ginContext.JSON(http.StatusCreated, tenantAssetResponse{
			AssetID: metadata.AssetID, MIMEType: metadata.MIMEType, SizeBytes: metadata.SizeBytes,
			State: metadata.State, CreatedAt: metadata.CreatedAt, ExpiresAt: metadata.ExpiresAt,
		})
	}
}

func validTenantAssetUploadHeaders(request *http.Request) bool {
	allowedProxyHeader := http.CanonicalHeaderKey(llmproxycontract.HeaderRequestTimeoutSeconds)
	for headerName := range request.Header {
		canonicalName := http.CanonicalHeaderKey(headerName)
		if strings.HasPrefix(canonicalName, "X-Llm-Proxy-") && canonicalName != allowedProxyHeader {
			return false
		}
	}
	return true
}

func tenantAssetDeleteHandler(store *tenantAssetStore) gin.HandlerFunc {
	return func(ginContext *gin.Context) {
		if deleteError := store.delete(authenticatedTenantFromContext(ginContext), ginContext.Param("asset_id")); deleteError != nil {
			writeTenantAssetError(ginContext, deleteError)
			return
		}
		ginContext.Status(http.StatusNoContent)
	}
}

func writeTenantAssetError(ginContext *gin.Context, assetError error) {
	statusCode := http.StatusBadRequest
	code := errAssetInvalid.Error()
	switch {
	case errors.Is(assetError, errAssetNotFound):
		statusCode, code = http.StatusNotFound, errAssetNotFound.Error()
	case errors.Is(assetError, errAssetExpired):
		statusCode, code = http.StatusGone, errAssetExpired.Error()
	case errors.Is(assetError, errAssetDeleted):
		statusCode, code = http.StatusGone, errAssetDeleted.Error()
	case errors.Is(assetError, errAssetMIMEMismatch):
		code = errAssetMIMEMismatch.Error()
	case errors.Is(assetError, errAssetTooLarge):
		statusCode, code = http.StatusRequestEntityTooLarge, errAssetTooLarge.Error()
	case errors.Is(assetError, errAssetStore):
		statusCode, code = http.StatusInternalServerError, errAssetStore.Error()
	}
	ginContext.JSON(statusCode, tenantAssetErrorEnvelope{Error: tenantAssetErrorDetail{Code: code}})
}

func isTenantAssetError(assetError error) bool {
	return errors.Is(assetError, errAssetInvalid) || errors.Is(assetError, errAssetNotFound) || errors.Is(assetError, errAssetExpired) || errors.Is(assetError, errAssetDeleted) || errors.Is(assetError, errAssetMIMEMismatch) || errors.Is(assetError, errAssetTooLarge) || errors.Is(assetError, errAssetStore)
}
