package proxy

import (
	"bytes"
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
	assetMetadataVersion = 1
	assetStateAvailable  = "available"
	assetStateDeleted    = "deleted"
	assetFileMode        = 0o600
	assetDirectoryMode   = 0o700
)

var (
	errAssetInvalid        = errors.New("asset_invalid")
	errAssetNotFound       = errors.New("asset_not_found")
	errAssetExpired        = errors.New("asset_expired")
	errAssetDeleted        = errors.New("asset_deleted")
	errAssetMIMEMismatch   = errors.New("asset_mime_mismatch")
	errAssetDigestMismatch = errors.New("asset_digest_mismatch")
	errAssetTooLarge       = errors.New("asset_too_large")
	errAssetStore          = errors.New("asset_store_error")

	assetIdentifierPattern = regexp.MustCompile(`^ast_[0-9a-f]{32}$`)
)

type tenantAssetMetadata struct {
	Version   int        `json:"version"`
	AssetID   string     `json:"asset_id"`
	TenantID  string     `json:"tenant_id"`
	MIMEType  string     `json:"mime_type"`
	SizeBytes int64      `json:"size_bytes"`
	SHA256    string     `json:"sha256"`
	State     string     `json:"state"`
	CreatedAt time.Time  `json:"created_at"`
	ExpiresAt time.Time  `json:"expires_at"`
	DeletedAt *time.Time `json:"deleted_at,omitempty"`
}

type tenantAssetResponse struct {
	AssetID   string    `json:"asset_id"`
	MIMEType  string    `json:"mime_type"`
	SizeBytes int64     `json:"size_bytes"`
	SHA256    string    `json:"sha256"`
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
	assetStat       = func(file *os.File) (os.FileInfo, error) { return file.Stat() }
	assetSeek       = func(file *os.File, offset int64, whence int) (int64, error) { return file.Seek(offset, whence) }
	assetWrite      = func(file *os.File, data []byte) (int, error) { return file.Write(data) }
)

func newTenantAssetStore(root string, maxAssetBytes int64, retentionSeconds int) *tenantAssetStore {
	return &tenantAssetStore{
		root:          root,
		maxAssetBytes: maxAssetBytes,
		retention:     time.Duration(retentionSeconds) * time.Second,
		now:           time.Now,
	}
}

func (store *tenantAssetStore) upload(requestTenant tenant, mimeType string, expectedDigest string, source io.Reader) (tenantAssetMetadata, error) {
	if !supportedMessageMediaMIME(mimeType) || !canonicalSHA256(expectedDigest) {
		return tenantAssetMetadata{}, errAssetInvalid
	}
	store.mutex.Lock()
	defer store.mutex.Unlock()
	if directoryError := assetMkdirAll(store.root, assetDirectoryMode); directoryError != nil {
		return tenantAssetMetadata{}, fmt.Errorf("%w: create directory", errAssetStore)
	}
	if chmodError := assetChmod(store.root, assetDirectoryMode); chmodError != nil {
		return tenantAssetMetadata{}, fmt.Errorf("%w: set directory mode", errAssetStore)
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
	actualDigest := hex.EncodeToString(hasher.Sum(nil))
	if !bytes.Equal([]byte(actualDigest), []byte(expectedDigest)) {
		return tenantAssetMetadata{}, errAssetDigestMismatch
	}
	assetID := newAssetIdentifier()
	createdAt := store.now().UTC()
	metadata := tenantAssetMetadata{
		Version:   assetMetadataVersion,
		AssetID:   assetID,
		TenantID:  requestTenant.identifier.string(),
		MIMEType:  mimeType,
		SizeBytes: written,
		SHA256:    actualDigest,
		State:     assetStateAvailable,
		CreatedAt: createdAt,
		ExpiresAt: createdAt.Add(store.retention),
	}
	dataPath := store.dataPath(assetID)
	if renameError := assetRename(temporaryPath, dataPath); renameError != nil {
		return tenantAssetMetadata{}, fmt.Errorf("%w: publish data", errAssetStore)
	}
	if metadataError := store.writeMetadata(metadata); metadataError != nil {
		assetRemove(dataPath)
		return tenantAssetMetadata{}, metadataError
	}
	return metadata, nil
}

func (store *tenantAssetStore) resolve(requestTenant tenant, assetID string, expectedMIMEType string, expectedDigest string) (*tenantAssetReader, error) {
	if !assetIdentifierPattern.MatchString(assetID) || !supportedMessageMediaMIME(expectedMIMEType) || !canonicalSHA256(expectedDigest) {
		return nil, errAssetInvalid
	}
	store.mutex.Lock()
	defer store.mutex.Unlock()
	metadata, metadataError := store.readMetadata(assetID)
	if metadataError != nil {
		return nil, metadataError
	}
	if metadata.TenantID != requestTenant.identifier.string() {
		return nil, errAssetNotFound
	}
	if metadata.State == assetStateDeleted {
		return nil, errAssetDeleted
	}
	if !store.now().UTC().Before(metadata.ExpiresAt) {
		if removeError := assetRemove(store.dataPath(assetID)); removeError != nil && !errors.Is(removeError, os.ErrNotExist) {
			return nil, errAssetStore
		}
		return nil, errAssetExpired
	}
	if metadata.MIMEType != expectedMIMEType {
		return nil, errAssetMIMEMismatch
	}
	if metadata.SHA256 != expectedDigest {
		return nil, errAssetDigestMismatch
	}
	dataFile, openError := assetOpen(store.dataPath(assetID))
	if openError != nil {
		return nil, errAssetStore
	}
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
	if hex.EncodeToString(hasher.Sum(nil)) != metadata.SHA256 {
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
	if metadata.Version != assetMetadataVersion || metadata.AssetID != assetID || metadata.TenantID == constants.EmptyString || !supportedMessageMediaMIME(metadata.MIMEType) || metadata.SizeBytes <= 0 || !canonicalSHA256(metadata.SHA256) || metadata.CreatedAt.IsZero() || !metadata.ExpiresAt.After(metadata.CreatedAt) || !validState {
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
		metadata, uploadError := store.upload(
			authenticatedTenantFromContext(ginContext),
			strings.TrimSpace(ginContext.GetHeader(headerContentType)),
			strings.TrimSpace(ginContext.GetHeader(llmproxycontract.HeaderAssetSHA256)),
			ginContext.Request.Body,
		)
		if uploadError != nil {
			writeTenantAssetError(ginContext, uploadError)
			return
		}
		ginContext.JSON(http.StatusCreated, tenantAssetResponse{
			AssetID: metadata.AssetID, MIMEType: metadata.MIMEType, SizeBytes: metadata.SizeBytes,
			SHA256: metadata.SHA256, State: metadata.State, CreatedAt: metadata.CreatedAt, ExpiresAt: metadata.ExpiresAt,
		})
	}
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
	case errors.Is(assetError, errAssetDigestMismatch):
		code = errAssetDigestMismatch.Error()
	case errors.Is(assetError, errAssetTooLarge):
		statusCode, code = http.StatusRequestEntityTooLarge, errAssetTooLarge.Error()
	case errors.Is(assetError, errAssetStore):
		statusCode, code = http.StatusInternalServerError, errAssetStore.Error()
	}
	ginContext.JSON(statusCode, tenantAssetErrorEnvelope{Error: tenantAssetErrorDetail{Code: code}})
}

func isTenantAssetError(assetError error) bool {
	return errors.Is(assetError, errAssetInvalid) || errors.Is(assetError, errAssetNotFound) || errors.Is(assetError, errAssetExpired) || errors.Is(assetError, errAssetDeleted) || errors.Is(assetError, errAssetMIMEMismatch) || errors.Is(assetError, errAssetDigestMismatch) || errors.Is(assetError, errAssetTooLarge) || errors.Is(assetError, errAssetStore)
}
