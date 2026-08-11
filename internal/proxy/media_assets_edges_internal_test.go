package proxy

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

var errAssetEdge = errors.New("asset edge failure")

type gatedAssetReader struct {
	reader  *bytes.Reader
	started chan struct{}
	release chan struct{}
	once    sync.Once
}

func (reader *gatedAssetReader) Read(data []byte) (int, error) {
	reader.once.Do(func() { close(reader.started) })
	<-reader.release
	return reader.reader.Read(data)
}

func TestTenantAssetUploadDoesNotBlockExistingAssetResolution(t *testing.T) {
	requestTenant, tenantError := newTenant(TenantConfiguration{ID: "asset-concurrency", Secret: "secret"})
	if tenantError != nil {
		t.Fatalf("new tenant: %v", tenantError)
	}
	store := newTenantAssetStore(t.TempDir(), 1024, 60)
	existingData := []byte("existing")
	existingDigestBytes := sha256.Sum256(existingData)
	existingDigest := hex.EncodeToString(existingDigestBytes[:])
	existing, uploadError := store.upload(requestTenant, "image/png", existingDigest, bytes.NewReader(existingData))
	if uploadError != nil {
		t.Fatalf("existing upload: %v", uploadError)
	}

	slowData := []byte("slow upload")
	slowDigestBytes := sha256.Sum256(slowData)
	slowDigest := hex.EncodeToString(slowDigestBytes[:])
	started := make(chan struct{})
	release := make(chan struct{})
	uploadComplete := make(chan error, 1)
	go func() {
		_, slowUploadError := store.upload(requestTenant, "image/png", slowDigest, &gatedAssetReader{reader: bytes.NewReader(slowData), started: started, release: release})
		uploadComplete <- slowUploadError
	}()
	<-started
	resolveComplete := make(chan error, 1)
	go func() {
		reader, resolveError := store.resolve(requestTenant, existing.AssetID, existing.MIMEType, existing.SHA256)
		if resolveError == nil {
			resolveError = reader.Close()
		}
		resolveComplete <- resolveError
	}()
	select {
	case resolveError := <-resolveComplete:
		if resolveError != nil {
			close(release)
			t.Fatalf("resolve during upload: %v", resolveError)
		}
	case <-time.After(2 * time.Second):
		close(release)
		t.Fatal("existing asset resolution blocked behind upload stream")
	}
	close(release)
	if slowUploadError := <-uploadComplete; slowUploadError != nil {
		t.Fatalf("slow upload: %v", slowUploadError)
	}
}

func TestTenantAssetStoreRecoversPersistedExpiryState(t *testing.T) {
	requestTenant, tenantError := newTenant(TenantConfiguration{ID: "asset-recovery", Secret: "secret"})
	if tenantError != nil {
		t.Fatalf("new tenant: %v", tenantError)
	}
	root := t.TempDir()
	now := time.Date(2026, 8, 11, 12, 0, 0, 0, time.UTC)
	store := newTenantAssetStore(root, 1024, 60)
	store.now = func() time.Time { return now }
	data := []byte("recovered")
	digestBytes := sha256.Sum256(data)
	digest := hex.EncodeToString(digestBytes[:])
	upload := func() tenantAssetMetadata {
		t.Helper()
		metadata, uploadError := store.upload(requestTenant, "image/png", digest, bytes.NewReader(data))
		if uploadError != nil {
			t.Fatalf("upload: %v", uploadError)
		}
		return metadata
	}
	available := upload()
	available.ExpiresAt = now.Add(10 * time.Minute)
	if metadataError := store.writeMetadata(available); metadataError != nil {
		t.Fatalf("available metadata: %v", metadataError)
	}
	expired := upload()
	expired.ExpiresAt = now.Add(time.Minute)
	if metadataError := store.writeMetadata(expired); metadataError != nil {
		t.Fatalf("expired metadata: %v", metadataError)
	}
	deleted := upload()
	if deleteError := store.delete(requestTenant, deleted.AssetID); deleteError != nil {
		t.Fatalf("delete: %v", deleteError)
	}
	if directoryError := os.Mkdir(filepath.Join(root, "ignored-directory"), assetDirectoryMode); directoryError != nil {
		t.Fatalf("directory: %v", directoryError)
	}
	if writeError := os.WriteFile(filepath.Join(root, "ignored.txt"), []byte("ignored"), assetFileMode); writeError != nil {
		t.Fatalf("nonmetadata: %v", writeError)
	}
	if writeError := os.WriteFile(filepath.Join(root, "not-an-asset.json"), []byte("ignored"), assetFileMode); writeError != nil {
		t.Fatalf("invalid metadata name: %v", writeError)
	}

	recoveredStore := newTenantAssetStore(root, 1024, 60)
	recoveredStore.now = func() time.Time { return now.Add(2 * time.Minute) }
	reader, resolveError := recoveredStore.resolve(requestTenant, available.AssetID, available.MIMEType, available.SHA256)
	if resolveError != nil {
		t.Fatalf("resolve available: %v", resolveError)
	}
	_ = reader.Close()
	if _, statError := os.Stat(recoveredStore.dataPath(expired.AssetID)); !errors.Is(statError, os.ErrNotExist) {
		t.Fatalf("expired data error=%v", statError)
	}
	if _, resolveError := recoveredStore.resolve(requestTenant, expired.AssetID, expired.MIMEType, expired.SHA256); !errors.Is(resolveError, errAssetExpired) {
		t.Fatalf("expired resolve error=%v", resolveError)
	}
	if _, resolveError := recoveredStore.resolve(requestTenant, deleted.AssetID, deleted.MIMEType, deleted.SHA256); !errors.Is(resolveError, errAssetDeleted) {
		t.Fatalf("deleted resolve error=%v", resolveError)
	}
}

func TestTenantAssetExpirationMaintenanceFailureContracts(t *testing.T) {
	originalCreateTemp := assetCreateTemp
	originalOpen := assetOpen
	originalRemove := assetRemove
	originalAfterFunc := assetAfterFunc
	timers := make([]*time.Timer, 0)
	reset := func() {
		assetCreateTemp = originalCreateTemp
		assetOpen = originalOpen
		assetRemove = originalRemove
		assetAfterFunc = originalAfterFunc
	}
	t.Cleanup(func() {
		reset()
		for _, timer := range timers {
			timer.Stop()
		}
	})

	requestTenant, tenantError := newTenant(TenantConfiguration{ID: "asset-maintenance", Secret: "secret"})
	if tenantError != nil {
		t.Fatalf("new tenant: %v", tenantError)
	}
	now := time.Date(2026, 8, 11, 12, 0, 0, 0, time.UTC)
	assetBytes := []byte("asset-maintenance")
	digestBytes := sha256.Sum256(assetBytes)
	digest := hex.EncodeToString(digestBytes[:])
	newStore := func(root string) *tenantAssetStore {
		store := newTenantAssetStore(root, 1024, 3600)
		store.now = func() time.Time { return now }
		return store
	}

	store := newStore(t.TempDir())
	metadata, uploadError := store.upload(requestTenant, "image/png", digest, bytes.NewReader(assetBytes))
	if uploadError != nil {
		t.Fatalf("upload: %v", uploadError)
	}
	assetAfterFunc = func(delay time.Duration, callback func()) *time.Timer {
		if delay != 0 || callback == nil {
			t.Fatalf("past expiration schedule delay=%s callback_nil=%t", delay, callback == nil)
		}
		timer := time.NewTimer(time.Hour)
		timers = append(timers, timer)
		return timer
	}
	pastMetadata := metadata
	pastMetadata.ExpiresAt = now.Add(-time.Second)
	store.scheduleExpirationLocked(pastMetadata)
	assetAfterFunc = originalAfterFunc

	failedStore := newStore(t.TempDir())
	failedStore.cleanupError = errAssetEdge
	failedStore.expireAsset(metadata.AssetID, metadata.ExpiresAt)
	if _, resolveError := failedStore.resolve(requestTenant, metadata.AssetID, "image/png", digest); !errors.Is(resolveError, errAssetEdge) {
		t.Fatalf("cleanup resolve error=%v", resolveError)
	}
	if deleteError := failedStore.delete(requestTenant, metadata.AssetID); !errors.Is(deleteError, errAssetEdge) {
		t.Fatalf("cleanup delete error=%v", deleteError)
	}

	missingStore := newStore(t.TempDir())
	missingStore.expireAsset("ast_00000000000000000000000000000000", now)

	assetOpen = func(string) (*os.File, error) { return nil, errAssetEdge }
	metadataFailureStore := newStore(t.TempDir())
	metadataFailureStore.expireAsset(metadata.AssetID, metadata.ExpiresAt)
	if !errors.Is(metadataFailureStore.cleanupError, errAssetStore) {
		t.Fatalf("expiration metadata error=%v", metadataFailureStore.cleanupError)
	}
	assetOpen = originalOpen

	store.expireAsset(metadata.AssetID, metadata.ExpiresAt.Add(time.Second))
	store.now = func() time.Time { return metadata.ExpiresAt.Add(time.Second) }
	assetRemove = func(string) error { return errAssetEdge }
	store.expireAsset(metadata.AssetID, metadata.ExpiresAt)
	if !errors.Is(store.cleanupError, errAssetStore) {
		t.Fatalf("expiration remove error=%v", store.cleanupError)
	}
	reset()

	uploadStore := newStore(t.TempDir())
	assetCreateTemp = func(directory string, pattern string) (*os.File, error) {
		uploadStore.cleanupError = errAssetEdge
		return originalCreateTemp(directory, pattern)
	}
	if _, uploadError := uploadStore.upload(requestTenant, "image/png", digest, bytes.NewReader(assetBytes)); !errors.Is(uploadError, errAssetEdge) {
		t.Fatalf("concurrent cleanup upload error=%v", uploadError)
	}
	reset()

	malformedRoot := t.TempDir()
	malformedID := "ast_11111111111111111111111111111111"
	if writeError := os.WriteFile(filepath.Join(malformedRoot, malformedID+".json"), []byte("{"), assetFileMode); writeError != nil {
		t.Fatalf("malformed metadata: %v", writeError)
	}
	malformedStore := newStore(malformedRoot)
	if initializationError := malformedStore.initializeLocked(); !errors.Is(initializationError, errAssetStore) {
		t.Fatalf("malformed initialization error=%v", initializationError)
	}

	reclaimRoot := t.TempDir()
	reclaimStore := newStore(reclaimRoot)
	reclaimMetadata, reclaimUploadError := reclaimStore.upload(requestTenant, "image/png", digest, bytes.NewReader(assetBytes))
	if reclaimUploadError != nil {
		t.Fatalf("reclaim upload: %v", reclaimUploadError)
	}
	recoveredStore := newStore(reclaimRoot)
	recoveredStore.now = func() time.Time { return reclaimMetadata.ExpiresAt.Add(time.Second) }
	assetRemove = func(string) error { return errAssetEdge }
	if initializationError := recoveredStore.initializeLocked(); !errors.Is(initializationError, errAssetStore) {
		t.Fatalf("reclaim initialization error=%v", initializationError)
	}
	reset()

	handlerStore := newStore(t.TempDir())
	request := httptest.NewRequest(http.MethodPost, "/assets", strings.NewReader("x"))
	request.Header.Set("Content-Type", "image/png")
	requestContext, cancelRequest := context.WithCancel(request.Context())
	cancelRequest()
	request = request.WithContext(requestContext)
	response := httptest.NewRecorder()
	ginContext, _ := gin.CreateTestContext(response)
	ginContext.Request = request
	ginContext.Set(contextKeyTenant, requestTenant)
	ginContext.Set(contextKeyRequestTimeoutState, &requestTimeoutState{})
	tenantAssetUploadHandler(handlerStore)(ginContext)
	if ginContext.Writer.Status() != statusClientClosedRequest {
		t.Fatalf("cancelled upload status=%d", ginContext.Writer.Status())
	}
}

func TestTenantAssetStoreFailureContracts(t *testing.T) {
	originalMkdirAll := assetMkdirAll
	originalChmod := assetChmod
	originalCreateTemp := assetCreateTemp
	originalCopy := assetCopy
	originalClose := assetClose
	originalRename := assetRename
	originalRemove := assetRemove
	originalOpen := assetOpen
	originalReadDir := assetReadDir
	originalStat := assetStat
	originalSeek := assetSeek
	originalWrite := assetWrite
	reset := func() {
		assetMkdirAll = originalMkdirAll
		assetChmod = originalChmod
		assetCreateTemp = originalCreateTemp
		assetCopy = originalCopy
		assetClose = originalClose
		assetRename = originalRename
		assetRemove = originalRemove
		assetOpen = originalOpen
		assetReadDir = originalReadDir
		assetStat = originalStat
		assetSeek = originalSeek
		assetWrite = originalWrite
	}
	t.Cleanup(reset)

	requestTenant, tenantError := newTenant(TenantConfiguration{ID: "asset-edge", Secret: "secret"})
	if tenantError != nil {
		t.Fatalf("new tenant: %v", tenantError)
	}
	assetBytes := []byte("asset-edge-bytes")
	digestBytes := sha256.Sum256(assetBytes)
	digest := hex.EncodeToString(digestBytes[:])
	newStore := func() *tenantAssetStore {
		store := newTenantAssetStore(t.TempDir(), int64(len(assetBytes)), 60)
		store.now = func() time.Time { return time.Date(2026, 8, 11, 12, 0, 0, 0, time.UTC) }
		return store
	}
	requireUploadError := func(store *tenantAssetStore, source io.Reader, mimeType string, expectedDigest string, expected error) {
		t.Helper()
		_, uploadError := store.upload(requestTenant, mimeType, expectedDigest, source)
		if !errors.Is(uploadError, expected) {
			t.Fatalf("upload error=%v want=%v", uploadError, expected)
		}
	}

	requireUploadError(newStore(), bytes.NewReader(assetBytes), "text/plain", digest, errAssetInvalid)

	store := newStore()
	assetMkdirAll = func(string, os.FileMode) error { return errAssetEdge }
	requireUploadError(store, bytes.NewReader(assetBytes), "image/png", digest, errAssetStore)
	reset()

	store = newStore()
	assetChmod = func(string, os.FileMode) error { return errAssetEdge }
	requireUploadError(store, bytes.NewReader(assetBytes), "image/png", digest, errAssetStore)
	reset()

	store = newStore()
	assetReadDir = func(string) ([]os.DirEntry, error) { return nil, errAssetEdge }
	requireUploadError(store, bytes.NewReader(assetBytes), "image/png", digest, errAssetStore)
	reset()

	store = newStore()
	assetCreateTemp = func(string, string) (*os.File, error) { return nil, errAssetEdge }
	requireUploadError(store, bytes.NewReader(assetBytes), "image/png", digest, errAssetStore)
	reset()

	store = newStore()
	chmodCalls := 0
	assetChmod = func(path string, mode os.FileMode) error {
		chmodCalls++
		if chmodCalls == 2 {
			return errAssetEdge
		}
		return originalChmod(path, mode)
	}
	requireUploadError(store, bytes.NewReader(assetBytes), "image/png", digest, errAssetStore)
	reset()

	store = newStore()
	assetCopy = func(io.Writer, io.Reader) (int64, error) { return 0, errAssetEdge }
	requireUploadError(store, bytes.NewReader(assetBytes), "image/png", digest, errAssetStore)
	reset()

	store = newStore()
	closeCalls := 0
	assetClose = func(file *os.File) error {
		closeCalls++
		closeError := originalClose(file)
		if closeCalls == 1 {
			return errAssetEdge
		}
		return closeError
	}
	requireUploadError(store, bytes.NewReader(assetBytes), "image/png", digest, errAssetStore)
	reset()

	requireUploadError(newStore(), bytes.NewReader(nil), "image/png", hex.EncodeToString(sha256.New().Sum(nil)), errAssetInvalid)
	tooLargeStore := newStore()
	tooLargeStore.maxAssetBytes--
	requireUploadError(tooLargeStore, bytes.NewReader(assetBytes), "image/png", digest, errAssetTooLarge)
	requireUploadError(newStore(), bytes.NewReader(assetBytes), "image/png", strings.Repeat("0", 64), errAssetDigestMismatch)

	store = newStore()
	assetRename = func(string, string) error { return errAssetEdge }
	requireUploadError(store, bytes.NewReader(assetBytes), "image/png", digest, errAssetStore)
	reset()

	store = newStore()
	renameCalls := 0
	assetRename = func(oldPath string, newPath string) error {
		renameCalls++
		if renameCalls == 2 {
			return errAssetEdge
		}
		return originalRename(oldPath, newPath)
	}
	requireUploadError(store, bytes.NewReader(assetBytes), "image/png", digest, errAssetStore)
	reset()
}

func TestTenantAssetMetadataAndResolutionFailureContracts(t *testing.T) {
	originalChmod := assetChmod
	originalCreateTemp := assetCreateTemp
	originalCopy := assetCopy
	originalClose := assetClose
	originalRename := assetRename
	originalRemove := assetRemove
	originalOpen := assetOpen
	originalStat := assetStat
	originalSeek := assetSeek
	originalWrite := assetWrite
	reset := func() {
		assetChmod = originalChmod
		assetCreateTemp = originalCreateTemp
		assetCopy = originalCopy
		assetClose = originalClose
		assetRename = originalRename
		assetRemove = originalRemove
		assetOpen = originalOpen
		assetStat = originalStat
		assetSeek = originalSeek
		assetWrite = originalWrite
	}
	t.Cleanup(reset)

	requestTenant, _ := newTenant(TenantConfiguration{ID: "asset-owner", Secret: "secret"})
	foreignTenant, _ := newTenant(TenantConfiguration{ID: "asset-foreign", Secret: "other"})
	now := time.Date(2026, 8, 11, 12, 0, 0, 0, time.UTC)
	store := newTenantAssetStore(t.TempDir(), 1024, 60)
	store.now = func() time.Time { return now }
	assetBytes := []byte("resolved-asset")
	digestBytes := sha256.Sum256(assetBytes)
	digest := hex.EncodeToString(digestBytes[:])
	metadata, uploadError := store.upload(requestTenant, "image/png", digest, bytes.NewReader(assetBytes))
	if uploadError != nil {
		t.Fatalf("upload: %v", uploadError)
	}

	if _, resolveError := store.resolve(requestTenant, "bad", "image/png", digest); !errors.Is(resolveError, errAssetInvalid) {
		t.Fatalf("invalid resolve error=%v", resolveError)
	}
	if _, resolveError := store.resolve(requestTenant, "ast_00000000000000000000000000000000", "image/png", digest); !errors.Is(resolveError, errAssetNotFound) {
		t.Fatalf("missing resolve error=%v", resolveError)
	}
	if _, resolveError := store.resolve(foreignTenant, metadata.AssetID, "image/png", digest); !errors.Is(resolveError, errAssetNotFound) {
		t.Fatalf("foreign resolve error=%v", resolveError)
	}

	metadataPath := store.metadataPath(metadata.AssetID)
	validMetadataBytes, readError := os.ReadFile(metadataPath)
	if readError != nil {
		t.Fatalf("read metadata: %v", readError)
	}
	if writeError := os.WriteFile(metadataPath, []byte("{"), assetFileMode); writeError != nil {
		t.Fatalf("write malformed metadata: %v", writeError)
	}
	if _, resolveError := store.resolve(requestTenant, metadata.AssetID, "image/png", digest); !errors.Is(resolveError, errAssetStore) {
		t.Fatalf("malformed metadata error=%v", resolveError)
	}
	if writeError := os.WriteFile(metadataPath, append(validMetadataBytes, []byte("{}")...), assetFileMode); writeError != nil {
		t.Fatalf("write trailing metadata: %v", writeError)
	}
	if _, resolveError := store.resolve(requestTenant, metadata.AssetID, "image/png", digest); !errors.Is(resolveError, errAssetStore) {
		t.Fatalf("trailing metadata error=%v", resolveError)
	}
	invalidMetadata := bytes.Replace(validMetadataBytes, []byte(`"version":1`), []byte(`"version":2`), 1)
	if writeError := os.WriteFile(metadataPath, invalidMetadata, assetFileMode); writeError != nil {
		t.Fatalf("write invalid metadata: %v", writeError)
	}
	if _, resolveError := store.resolve(requestTenant, metadata.AssetID, "image/png", digest); !errors.Is(resolveError, errAssetStore) {
		t.Fatalf("invalid metadata error=%v", resolveError)
	}
	if writeError := os.WriteFile(metadataPath, validMetadataBytes, assetFileMode); writeError != nil {
		t.Fatalf("restore metadata: %v", writeError)
	}

	assetOpen = func(path string) (*os.File, error) {
		if path == metadataPath {
			return nil, errAssetEdge
		}
		return originalOpen(path)
	}
	if _, resolveError := store.resolve(requestTenant, metadata.AssetID, "image/png", digest); !errors.Is(resolveError, errAssetStore) {
		t.Fatalf("metadata open error=%v", resolveError)
	}
	reset()

	changed := metadata
	changed.State = "processing"
	if metadataError := store.writeMetadata(changed); metadataError != nil {
		t.Fatalf("write state metadata: %v", metadataError)
	}
	if _, resolveError := store.resolve(requestTenant, metadata.AssetID, "image/png", digest); !errors.Is(resolveError, errAssetStore) {
		t.Fatalf("state error=%v", resolveError)
	}
	if writeError := os.WriteFile(metadataPath, validMetadataBytes, assetFileMode); writeError != nil {
		t.Fatalf("restore metadata: %v", writeError)
	}

	if _, resolveError := store.resolve(requestTenant, metadata.AssetID, "image/jpeg", digest); !errors.Is(resolveError, errAssetMIMEMismatch) {
		t.Fatalf("MIME error=%v", resolveError)
	}
	if _, resolveError := store.resolve(requestTenant, metadata.AssetID, "image/png", strings.Repeat("0", 64)); !errors.Is(resolveError, errAssetDigestMismatch) {
		t.Fatalf("digest error=%v", resolveError)
	}

	dataPath := store.dataPath(metadata.AssetID)
	assetOpen = func(path string) (*os.File, error) {
		if path == dataPath {
			return nil, errAssetEdge
		}
		return originalOpen(path)
	}
	if _, resolveError := store.resolve(requestTenant, metadata.AssetID, "image/png", digest); !errors.Is(resolveError, errAssetStore) {
		t.Fatalf("data open error=%v", resolveError)
	}
	reset()

	assetStat = func(*os.File) (os.FileInfo, error) { return nil, errAssetEdge }
	if _, resolveError := store.resolve(requestTenant, metadata.AssetID, "image/png", digest); !errors.Is(resolveError, errAssetStore) {
		t.Fatalf("stat error=%v", resolveError)
	}
	reset()

	assetCopy = func(io.Writer, io.Reader) (int64, error) { return 0, errAssetEdge }
	if _, resolveError := store.resolve(requestTenant, metadata.AssetID, "image/png", digest); !errors.Is(resolveError, errAssetStore) {
		t.Fatalf("hash error=%v", resolveError)
	}
	reset()

	if writeError := os.WriteFile(dataPath, bytes.Repeat([]byte("x"), len(assetBytes)), assetFileMode); writeError != nil {
		t.Fatalf("write altered data: %v", writeError)
	}
	if _, resolveError := store.resolve(requestTenant, metadata.AssetID, "image/png", digest); !errors.Is(resolveError, errAssetStore) {
		t.Fatalf("hash mismatch error=%v", resolveError)
	}
	if writeError := os.WriteFile(dataPath, assetBytes, assetFileMode); writeError != nil {
		t.Fatalf("restore data: %v", writeError)
	}

	assetSeek = func(*os.File, int64, int) (int64, error) { return 0, errAssetEdge }
	if _, resolveError := store.resolve(requestTenant, metadata.AssetID, "image/png", digest); !errors.Is(resolveError, errAssetStore) {
		t.Fatalf("seek error=%v", resolveError)
	}
	reset()

	reader, resolveError := store.resolve(requestTenant, metadata.AssetID, "image/png", digest)
	if resolveError != nil {
		t.Fatalf("valid resolve: %v", resolveError)
	}
	if closeError := reader.Close(); closeError != nil {
		t.Fatalf("close resolved asset: %v", closeError)
	}
	expiringMetadata, uploadError := store.upload(requestTenant, "image/png", digest, bytes.NewReader(assetBytes))
	if uploadError != nil {
		t.Fatalf("upload expiring asset: %v", uploadError)
	}
	store.now = func() time.Time { return now.Add(2 * time.Minute) }
	assetRemove = func(string) error { return errAssetEdge }
	if _, resolveError := store.resolve(requestTenant, expiringMetadata.AssetID, "image/png", digest); !errors.Is(resolveError, errAssetStore) {
		t.Fatalf("expired resolve remove error=%v", resolveError)
	}
	reset()
	store.now = func() time.Time { return now }

	writeMetadata := func(configure func()) {
		t.Helper()
		configure()
		if metadataError := store.writeMetadata(metadata); !errors.Is(metadataError, errAssetStore) {
			t.Fatalf("metadata error=%v", metadataError)
		}
		reset()
	}
	writeMetadata(func() { assetCreateTemp = func(string, string) (*os.File, error) { return nil, errAssetEdge } })
	writeMetadata(func() { assetChmod = func(string, os.FileMode) error { return errAssetEdge } })
	writeMetadata(func() { assetWrite = func(*os.File, []byte) (int, error) { return 0, errAssetEdge } })
	writeMetadata(func() {
		assetClose = func(file *os.File) error {
			_ = originalClose(file)
			return errAssetEdge
		}
	})
	writeMetadata(func() { assetRename = func(string, string) error { return errAssetEdge } })
}

func TestTenantAssetDeletionAndHTTPErrorContracts(t *testing.T) {
	requestTenant, _ := newTenant(TenantConfiguration{ID: "asset-delete", Secret: "secret"})
	foreignTenant, _ := newTenant(TenantConfiguration{ID: "asset-foreign", Secret: "other"})
	store := newTenantAssetStore(t.TempDir(), 128, 60)
	now := time.Date(2026, 8, 11, 12, 0, 0, 0, time.UTC)
	store.now = func() time.Time { return now }
	data := []byte("delete-asset")
	digestBytes := sha256.Sum256(data)
	digest := hex.EncodeToString(digestBytes[:])
	upload := func() tenantAssetMetadata {
		metadata, uploadError := store.upload(requestTenant, "image/png", digest, bytes.NewReader(data))
		if uploadError != nil {
			t.Fatalf("upload: %v", uploadError)
		}
		return metadata
	}

	if deleteError := store.delete(requestTenant, "bad"); !errors.Is(deleteError, errAssetNotFound) {
		t.Fatalf("invalid delete error=%v", deleteError)
	}
	if deleteError := store.delete(requestTenant, "ast_00000000000000000000000000000000"); !errors.Is(deleteError, errAssetNotFound) {
		t.Fatalf("missing delete error=%v", deleteError)
	}
	metadata := upload()
	if deleteError := store.delete(foreignTenant, metadata.AssetID); !errors.Is(deleteError, errAssetNotFound) {
		t.Fatalf("foreign delete error=%v", deleteError)
	}
	if deleteError := store.delete(requestTenant, metadata.AssetID); deleteError != nil {
		t.Fatalf("delete: %v", deleteError)
	}
	if deleteError := store.delete(requestTenant, metadata.AssetID); !errors.Is(deleteError, errAssetDeleted) {
		t.Fatalf("second delete error=%v", deleteError)
	}

	originalRemove := assetRemove
	originalWrite := assetWrite
	t.Cleanup(func() {
		assetRemove = originalRemove
		assetWrite = originalWrite
	})
	metadata = upload()
	assetRemove = func(string) error { return errAssetEdge }
	if deleteError := store.delete(requestTenant, metadata.AssetID); !errors.Is(deleteError, errAssetStore) {
		t.Fatalf("remove delete error=%v", deleteError)
	}
	assetRemove = originalRemove

	metadata = upload()
	if deleteError := store.delete(requestTenant, metadata.AssetID); deleteError != nil {
		t.Fatalf("delete before repeated failure: %v", deleteError)
	}
	assetRemove = func(string) error { return errAssetEdge }
	if deleteError := store.delete(requestTenant, metadata.AssetID); !errors.Is(deleteError, errAssetStore) {
		t.Fatalf("repeated delete remove error=%v", deleteError)
	}
	assetRemove = originalRemove

	metadata = upload()
	assetWrite = func(*os.File, []byte) (int, error) { return 0, errAssetEdge }
	if deleteError := store.delete(requestTenant, metadata.AssetID); !errors.Is(deleteError, errAssetStore) {
		t.Fatalf("metadata delete error=%v", deleteError)
	}
	assetWrite = originalWrite

	metadata = upload()
	store.now = func() time.Time { return now.Add(2 * time.Minute) }
	assetRemove = func(string) error { return errAssetEdge }
	if deleteError := store.delete(requestTenant, metadata.AssetID); !errors.Is(deleteError, errAssetStore) {
		t.Fatalf("expired remove error=%v", deleteError)
	}
	assetRemove = originalRemove
	if deleteError := store.delete(requestTenant, metadata.AssetID); !errors.Is(deleteError, errAssetExpired) {
		t.Fatalf("expired delete error=%v", deleteError)
	}

	for _, assetError := range []error{errAssetInvalid, errAssetNotFound, errAssetExpired, errAssetDeleted, errAssetMIMEMismatch, errAssetDigestMismatch, errAssetTooLarge, errAssetStore} {
		response := httptest.NewRecorder()
		ginContext, _ := gin.CreateTestContext(response)
		writeTenantAssetError(ginContext, assetError)
		if response.Code == 0 || !isTenantAssetError(assetError) {
			t.Fatalf("asset error=%v status=%d", assetError, response.Code)
		}
	}
	if isTenantAssetError(errAssetEdge) {
		t.Fatal("unexpected asset error classification")
	}
	if statusCodeForError(errAssetNotFound) != http.StatusNotFound || statusCodeForError(errAssetDeleted) != http.StatusGone {
		t.Fatal("asset status mapping mismatch")
	}

	handlerStore := newTenantAssetStore(t.TempDir(), 4, 60)
	router := gin.New()
	router.Use(func(ginContext *gin.Context) {
		ginContext.Set(contextKeyTenant, requestTenant)
		ginContext.Next()
	})
	timeoutPolicy, timeoutPolicyError := newRequestTimeoutPolicy(5, 5)
	if timeoutPolicyError != nil {
		t.Fatalf("timeout policy: %v", timeoutPolicyError)
	}
	router.POST("/assets", requestTimeoutHandler(timeoutPolicy, nil, tenantAssetUploadHandler(handlerStore)))
	router.DELETE("/assets/:asset_id", tenantAssetDeleteHandler(handlerStore))
	tooLargeRequest := httptest.NewRequest(http.MethodPost, "/assets", strings.NewReader("12345"))
	tooLargeRequest.Header.Set("Content-Type", "image/png")
	tooLargeRequest.Header.Set("X-Asset-SHA256", strings.Repeat("0", 64))
	tooLargeResponse := httptest.NewRecorder()
	router.ServeHTTP(tooLargeResponse, tooLargeRequest)
	if tooLargeResponse.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("upload status=%d", tooLargeResponse.Code)
	}
	invalidRequest := httptest.NewRequest(http.MethodPost, "/assets", strings.NewReader("x"))
	invalidResponse := httptest.NewRecorder()
	router.ServeHTTP(invalidResponse, invalidRequest)
	if invalidResponse.Code != http.StatusBadRequest {
		t.Fatalf("invalid upload status=%d", invalidResponse.Code)
	}
	deleteResponse := httptest.NewRecorder()
	router.ServeHTTP(deleteResponse, httptest.NewRequest(http.MethodDelete, "/assets/bad", nil))
	if deleteResponse.Code != http.StatusNotFound {
		t.Fatalf("invalid delete status=%d", deleteResponse.Code)
	}
}

func TestMediaLimitAndMessageMediaEdgeContracts(t *testing.T) {
	maximumValue := int64(math.MaxInt64)
	overflowCatalog := ModelCatalog{Offerings: []ProviderOffering{{MediaLimits: []CatalogMediaLimit{{ID: CatalogMediaLimitIDInlineRequestBytes, Status: CatalogMediaLimitStatusBounded, Value: &maximumValue}}}}}
	if maximumV2RequestBytes(1, overflowCatalog) != math.MaxInt64 || maximumV2RequestBytes(7, ModelCatalog{}) != 7 {
		t.Fatal("v2 request bound mismatch")
	}
	if _, configError := validateConfig(Configuration{AssetStorePath: "relative"}); configError == nil {
		t.Fatal("relative asset store path accepted")
	}
	value := int64(1)
	validSource := "https://example.com/limits"
	validDate := "2026-08-11"
	base := CatalogMediaLimit{ID: CatalogMediaLimitIDInlineRequestBytes, MediaType: CatalogMediaLimitTypeAll, Transport: CatalogMediaTransportInline, Status: CatalogMediaLimitStatusBounded, Value: &value, Unit: CatalogMediaLimitUnitBytes, Scope: CatalogMediaLimitScopeRequestEncodedBytes, Source: validSource, LastVerified: validDate}
	if validationError := validateCatalogMediaLimits([]CatalogMediaLimit{base}, nil, "limits"); validationError == nil {
		t.Fatal("expected limits without inputs rejection")
	}
	invalidLimits := []CatalogMediaLimit{
		{ID: "INVALID", MediaType: "all", Transport: "inline", Status: "bounded", Value: &value, Unit: "bytes", Scope: "request_encoded_bytes", Source: validSource, LastVerified: validDate},
		base,
		{ID: "other", MediaType: "video", Transport: "inline", Status: "bounded", Value: &value, Unit: "bytes", Scope: "request", Source: validSource, LastVerified: validDate},
		{ID: "other", MediaType: "image", Transport: "wire", Status: "bounded", Value: &value, Unit: "bytes", Scope: "request", Source: validSource, LastVerified: validDate},
		{ID: "other", MediaType: "image", Transport: "inline", Status: "bounded", Value: &value, Unit: "items", Scope: "request", Source: validSource, LastVerified: validDate},
		{ID: "other", MediaType: "image", Transport: "inline", Status: "bounded", Value: &value, Unit: "bytes", Scope: "turn", Source: validSource, LastVerified: validDate},
		{ID: "other", MediaType: "image", Transport: "inline", Status: "unbounded", Value: &value, Unit: "bytes", Scope: "request", Source: validSource, LastVerified: validDate},
		{ID: "other", MediaType: "image", Transport: "inline", Status: "invalid", Unit: "bytes", Scope: "request", Source: validSource, LastVerified: validDate},
		{ID: "other", MediaType: "image", Transport: "inline", Status: "bounded", Value: &value, Unit: "bytes", Scope: "request", Source: "http://example.com", LastVerified: validDate},
		{ID: "other", MediaType: "image", Transport: "inline", Status: "bounded", Value: &value, Unit: "bytes", Scope: "request", Source: validSource, LastVerified: "bad"},
	}
	for limitIndex, invalidLimit := range invalidLimits {
		limits := []CatalogMediaLimit{invalidLimit}
		if limitIndex == 1 {
			limits = []CatalogMediaLimit{base, base}
		}
		if validationError := validateCatalogMediaLimits(limits, []string{"image"}, "limits"); validationError == nil {
			t.Fatalf("invalid limit index=%d accepted", limitIndex)
		}
	}
	if validationError := validateCatalogMediaLimits([]CatalogMediaLimit{base}, []string{"image"}, "limits"); validationError == nil {
		t.Fatal("expected missing required limit rejection")
	}

	assetID := " ast_0123456789abcdef0123456789abcdef"
	if _, mediaError := newMessageMedia(chatMessageAttachmentPayload{Type: "image", MIMEType: "image/png", AssetID: &assetID, SHA256: strings.Repeat("0", 64)}, tenant{}, newTenantAssetStore(t.TempDir(), 10, 60)); mediaError == nil {
		t.Fatal("expected noncanonical asset id rejection")
	}
	dataFile, fileError := os.CreateTemp(t.TempDir(), "closed-asset")
	if fileError != nil {
		t.Fatalf("create asset file: %v", fileError)
	}
	_ = dataFile.Close()
	media := messageMedia{asset: &tenantAssetReader{file: dataFile}, sizeBytes: 1}
	if _, readerError := media.reader(); readerError == nil {
		t.Fatal("expected closed asset reader rejection")
	}
	if _, bytesError := media.bytes(); bytesError == nil {
		t.Fatal("expected closed asset byte rejection")
	}
	shortFile, shortError := os.CreateTemp(t.TempDir(), "short-asset")
	if shortError != nil {
		t.Fatalf("create short asset: %v", shortError)
	}
	defer shortFile.Close()
	media = messageMedia{asset: &tenantAssetReader{file: shortFile}, sizeBytes: 1}
	if _, bytesError := media.bytes(); bytesError == nil {
		t.Fatal("expected short asset byte rejection")
	}
}
