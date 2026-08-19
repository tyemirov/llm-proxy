package proxy

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

var errStructuredRequestTestIO = errors.New("structured request test I/O failure")

func TestStructuredRequestDirectoryAndRecoveryFailures(testingInstance *testing.T) {
	reset := captureStructuredRequestFileOperations()
	testingInstance.Cleanup(reset)

	structuredRequestLstat = func(string) (os.FileInfo, error) { return nil, errStructuredRequestTestIO }
	if _, storeError := newStructuredRequestStore(testingInstance.TempDir(), 10); !errors.Is(storeError, errStructuredRequestTestIO) {
		testingInstance.Fatalf("store lstat error=%v", storeError)
	}
	if directoryError := ensurePrivateDirectory("ignored"); !errors.Is(directoryError, errStructuredRequestTestIO) {
		testingInstance.Fatalf("directory lstat error=%v", directoryError)
	}
	reset()

	structuredRequestLstat = func(string) (os.FileInfo, error) { return nil, os.ErrNotExist }
	structuredRequestMkdirAll = func(string, fs.FileMode) error { return errStructuredRequestTestIO }
	if directoryError := ensurePrivateDirectory("ignored"); !errors.Is(directoryError, errStructuredRequestTestIO) {
		testingInstance.Fatalf("mkdir error=%v", directoryError)
	}
	reset()

	existingDirectory := testingInstance.TempDir()
	structuredRequestChmod = func(string, fs.FileMode) error { return errStructuredRequestTestIO }
	if directoryError := ensurePrivateDirectory(existingDirectory); !errors.Is(directoryError, errStructuredRequestTestIO) {
		testingInstance.Fatalf("chmod error=%v", directoryError)
	}
	reset()

	structuredRequestWalkDir = func(root string, visit fs.WalkDirFunc) error { return visit(root, nil, errStructuredRequestTestIO) }
	store := &structuredRequestStore{root: testingInstance.TempDir(), now: time.Now}
	if recoveryError := store.recoverInterrupted(); !errors.Is(recoveryError, errStructuredRequestTestIO) {
		testingInstance.Fatalf("walk error=%v", recoveryError)
	}
	reset()

	symlinkRoot := testingInstance.TempDir()
	structuredRoot := filepath.Join(symlinkRoot, structuredRequestDirectoryName)
	if makeError := os.MkdirAll(structuredRoot, 0o700); makeError != nil {
		testingInstance.Fatal(makeError)
	}
	if symlinkError := os.Symlink(symlinkRoot, filepath.Join(structuredRoot, "unsafe")); symlinkError != nil {
		testingInstance.Fatal(symlinkError)
	}
	if _, storeError := newStructuredRequestStore(symlinkRoot, 10); storeError == nil {
		testingInstance.Fatal("recovery symlink must fail")
	}

	invalidRoot := testingInstance.TempDir()
	invalidStructuredRoot := filepath.Join(invalidRoot, structuredRequestDirectoryName)
	if makeError := os.MkdirAll(invalidStructuredRoot, 0o700); makeError != nil {
		testingInstance.Fatal(makeError)
	}
	if writeError := os.WriteFile(filepath.Join(invalidStructuredRoot, "invalid.json"), []byte(`{`), 0o600); writeError != nil {
		testingInstance.Fatal(writeError)
	}
	if _, storeError := newStructuredRequestStore(invalidRoot, 10); storeError == nil {
		testingInstance.Fatal("invalid recovery record must fail")
	}

	timeRoot := testingInstance.TempDir()
	placeholder := filepath.Join(timeRoot, "record.json")
	if writeError := os.WriteFile(placeholder, []byte(`{}`), 0o600); writeError != nil {
		testingInstance.Fatal(writeError)
	}
	invalidTimeRecord := validStructuredRequestTestRecord()
	invalidTimeRecord.UpdatedAt = "not-a-time"
	timeStore := &structuredRequestStore{
		root: timeRoot, now: time.Now,
		read: func(string) (structuredRequestRecord, error) { return invalidTimeRecord, nil },
	}
	if recoveryError := timeStore.recoverInterrupted(); recoveryError == nil {
		testingInstance.Fatal("invalid recovery time must fail")
	}
}

func TestStructuredRequestPathAndReadFailures(testingInstance *testing.T) {
	reset := captureStructuredRequestFileOperations()
	testingInstance.Cleanup(reset)
	requestTenant := structuredTestTenant(testingInstance, "path-failure")
	intent := structuredRequestIntent("openai", "gpt", []byte(`{}`))

	root := testingInstance.TempDir()
	store := &structuredRequestStore{
		root: root, now: time.Now, read: readStructuredRequestRecord, publish: publishStructuredRequestRecord,
	}
	structuredRequestChmod = func(string, fs.FileMode) error { return errStructuredRequestTestIO }
	if _, _, beginError := store.begin(requestTenant, "key", intent, "openai", "gpt", "proxy"); !errors.Is(beginError, errStructuredRequestTestIO) {
		testingInstance.Fatalf("begin path error=%v", beginError)
	}
	reset()

	chmodCount := 0
	structuredRequestChmod = func(path string, mode fs.FileMode) error {
		chmodCount++
		if chmodCount == 2 {
			return errStructuredRequestTestIO
		}
		return os.Chmod(path, mode)
	}
	if _, pathError := store.recordPath(strings.Repeat("a", 64), strings.Repeat("b", 64), true); !errors.Is(pathError, errStructuredRequestTestIO) {
		testingInstance.Fatalf("tenant path error=%v", pathError)
	}
	reset()

	regularPath := filepath.Join(testingInstance.TempDir(), "record.json")
	if writeError := os.WriteFile(regularPath, []byte(`{}`), 0o600); writeError != nil {
		testingInstance.Fatal(writeError)
	}
	structuredRequestReadFile = func(string) ([]byte, error) { return nil, errStructuredRequestTestIO }
	if _, readError := readStructuredRequestRecord(regularPath); !errors.Is(readError, errStructuredRequestTestIO) {
		testingInstance.Fatalf("read file error=%v", readError)
	}
	reset()

	store, _ = newStructuredRequestStore(testingInstance.TempDir(), 10)
	_, _, _ = store.begin(requestTenant, "conflict", intent, "openai", "gpt", "proxy")
	if _, transitionError := store.transition(requestTenant, "conflict", strings.Repeat("f", 64), func(*structuredRequestRecord, time.Time) {}); !errors.Is(transitionError, errStructuredRequestConflict) {
		testingInstance.Fatalf("transition conflict=%v", transitionError)
	}

	_, _, _ = store.begin(requestTenant, "retry-publish", intent, "openai", "gpt", "proxy")
	if failError := store.fail(requestTenant, "retry-publish", intent, 502, "provider_failed"); failError != nil {
		testingInstance.Fatal(failError)
	}
	store.publish = func(string, structuredRequestRecord) error { return errStructuredRequestTestIO }
	if _, _, retryError := store.begin(requestTenant, "retry-publish", intent, "openai", "gpt", "proxy-retry"); !errors.Is(retryError, errStructuredRequestTestIO) {
		testingInstance.Fatalf("retry publish error=%v", retryError)
	}
}

func TestStructuredRequestAtomicPublishFailures(testingInstance *testing.T) {
	reset := captureStructuredRequestFileOperations()
	testingInstance.Cleanup(reset)
	record := validStructuredRequestTestRecord()

	runFailure := func(name string, mutate func(), expected error) {
		testingInstance.Run(name, func(subtest *testing.T) {
			reset()
			mutate()
			path := filepath.Join(subtest.TempDir(), "record.json")
			if publishError := publishStructuredRequestRecord(path, record); !errors.Is(publishError, expected) {
				subtest.Fatalf("publish error=%v", publishError)
			}
		})
	}
	runFailure("marshal", func() {
		structuredRequestMarshal = func(any, string, string) ([]byte, error) { return nil, errStructuredRequestTestIO }
	}, errStructuredRequestTestIO)
	runFailure("create", func() {
		structuredRequestCreateTemp = func(string, string) (*os.File, error) { return nil, errStructuredRequestTestIO }
	}, errStructuredRequestTestIO)
	runFailure("chmod", func() {
		structuredRequestFileChmod = func(*os.File, fs.FileMode) error { return errStructuredRequestTestIO }
	}, errStructuredRequestTestIO)
	runFailure("write", func() {
		structuredRequestFileWrite = func(*os.File, []byte) (int, error) { return 0, errStructuredRequestTestIO }
	}, errStructuredRequestTestIO)
	runFailure("sync", func() {
		structuredRequestFileSync = func(*os.File) error { return errStructuredRequestTestIO }
	}, errStructuredRequestTestIO)
	runFailure("close", func() {
		closeCount := 0
		structuredRequestFileClose = func(file *os.File) error {
			closeCount++
			if closeCount == 1 {
				return errStructuredRequestTestIO
			}
			return file.Close()
		}
	}, errStructuredRequestTestIO)
	runFailure("rename", func() {
		structuredRequestRename = func(string, string) error { return errStructuredRequestTestIO }
	}, errStructuredRequestTestIO)
	runFailure("directory open", func() {
		structuredRequestOpen = func(string) (*os.File, error) { return nil, errStructuredRequestTestIO }
	}, errStructuredRequestTestIO)
	reset()
}

func validStructuredRequestTestRecord() structuredRequestRecord {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	return structuredRequestRecord{
		Schema: structuredRequestRecordSchema, TenantSHA256: strings.Repeat("a", 64), IdempotencySHA256: strings.Repeat("b", 64),
		IntentSHA256: strings.Repeat("c", 64), Provider: "openai", Model: "gpt", ProxyRequestID: "proxy",
		State: structuredRequestStateNotDispatched, StartedAt: now, UpdatedAt: now,
	}
}

func captureStructuredRequestFileOperations() func() {
	originalLstat := structuredRequestLstat
	originalChmod := structuredRequestChmod
	originalMkdirAll := structuredRequestMkdirAll
	originalWalkDir := structuredRequestWalkDir
	originalRemove := structuredRequestRemove
	originalReadFile := structuredRequestReadFile
	originalMarshal := structuredRequestMarshal
	originalCreateTemp := structuredRequestCreateTemp
	originalFileChmod := structuredRequestFileChmod
	originalFileWrite := structuredRequestFileWrite
	originalFileSync := structuredRequestFileSync
	originalFileClose := structuredRequestFileClose
	originalRename := structuredRequestRename
	originalOpen := structuredRequestOpen
	return func() {
		structuredRequestLstat = originalLstat
		structuredRequestChmod = originalChmod
		structuredRequestMkdirAll = originalMkdirAll
		structuredRequestWalkDir = originalWalkDir
		structuredRequestRemove = originalRemove
		structuredRequestReadFile = originalReadFile
		structuredRequestMarshal = originalMarshal
		structuredRequestCreateTemp = originalCreateTemp
		structuredRequestFileChmod = originalFileChmod
		structuredRequestFileWrite = originalFileWrite
		structuredRequestFileSync = originalFileSync
		structuredRequestFileClose = originalFileClose
		structuredRequestRename = originalRename
		structuredRequestOpen = originalOpen
	}
}
