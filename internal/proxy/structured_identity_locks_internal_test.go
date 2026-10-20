package proxy

import (
	"errors"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestStructuredIdentityLockWaiterRetirement(t *testing.T) {
	var locks structuredIdentityLocks
	identity := structuredRequestIdentity{tenantDigest: "tenant", keyDigest: "key"}
	unlock := locks.lock(identity)
	acquired, release, finished := make(chan struct{}), make(chan struct{}), make(chan struct{})
	go func() { defer close(finished); unblock := locks.lock(identity); close(acquired); <-release; unblock() }()
	deadline := time.Now().Add(3 * time.Second)
	for {
		locks.mutex.Lock()
		references := locks.entries[identity].references
		locks.mutex.Unlock()
		if references == 2 {
			break
		}
		if time.Now().After(deadline) {
			unlock()
			close(release)
			<-finished
			t.Fatal("waiter was not registered")
		}
		runtime.Gosched()
	}
	select {
	case <-acquired:
		t.Fatal("same identity entered while holder active")
	default:
	}
	// A distinct tenant with the same key and a distinct key in the same tenant
	// can enter while this identity has both an active holder and a waiter.
	otherTenant := locks.lock(structuredRequestIdentity{tenantDigest: "other", keyDigest: "key"})
	otherTenant()
	otherKey := locks.lock(structuredRequestIdentity{tenantDigest: "tenant", keyDigest: "other"})
	otherKey()
	unlock()
	<-acquired
	locks.mutex.Lock()
	entry := locks.entries[identity]
	references := entry.references
	locks.mutex.Unlock()
	if references != 1 {
		t.Fatalf("active waiter references=%d", references)
	}
	close(release)
	<-finished
	locks.mutex.Lock()
	remaining := len(locks.entries)
	locks.mutex.Unlock()
	if remaining != 0 {
		t.Fatalf("retired identities=%d", remaining)
	}
}

func TestStructuredIdentityLockConcurrentReuse(t *testing.T) {
	var locks structuredIdentityLocks
	var active [4]atomic.Int64
	var violations atomic.Int64
	var workers sync.WaitGroup
	for worker := 0; worker < 32; worker++ {
		workers.Add(1)
		go func(index int) {
			defer workers.Done()
			for iteration := 0; iteration < 64; iteration++ {
				slot := index % len(active)
				unlock := locks.lock(structuredRequestIdentity{tenantDigest: "tenant", keyDigest: string(rune('a' + slot))})
				if active[slot].Add(1) != 1 {
					violations.Add(1)
				}
				runtime.Gosched()
				active[slot].Add(-1)
				unlock()
			}
		}(worker)
	}
	workers.Wait()
	if violations.Load() != 0 {
		t.Fatalf("same-identity concurrent holders=%d", violations.Load())
	}
	if len(locks.entries) != 0 {
		t.Fatalf("registry retained %d identities", len(locks.entries))
	}
}

func TestStructuredIdentityRecoveryGate(t *testing.T) {
	store := &structuredRequestStore{}
	unlock := store.lockRecord("tenant", "key")
	recovered := make(chan struct{})
	go func() { store.mu.Lock(); close(recovered); store.mu.Unlock() }()
	select {
	case <-recovered:
		t.Fatal("recovery overlapped active identity")
	default:
	}
	unlock()
	<-recovered
	next := store.lockRecord("tenant", "key")
	next()
}

func TestStructuredIdentityPublicationFailureRetiresLock(t *testing.T) {
	store, err := newStructuredRequestStore(t.TempDir(), 3600, nil)
	if err != nil {
		t.Fatal(err)
	}
	tenant := structuredTestTenant(t, "identity-failure")
	publish := store.publish
	failure := errors.New("controlled identity publication failure")
	store.publish = func(string, structuredRequestRecord) error { return failure }
	intent := structuredRequestIntent("openai", "gpt", []byte(`{}`))
	if _, _, err := store.begin(tenant, "key", intent, "openai", "gpt", "request"); !errors.Is(err, failure) {
		t.Fatalf("begin failure=%v", err)
	}
	if len(store.identities.entries) != 0 {
		t.Fatal("failed initial publication retained identity lock")
	}
	store.publish = publish
	if _, _, err := store.begin(tenant, "key", intent, "openai", "gpt", "request"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.claimDispatch(tenant, "key", intent); err != nil {
		t.Fatal(err)
	}
	store.publish = func(string, structuredRequestRecord) error { return failure }
	if err := store.succeed(tenant, "key", intent, `{"decision":"pass"}`); !errors.Is(err, failure) {
		t.Fatalf("success publication failure=%v", err)
	}
	if len(store.identities.entries) != 0 {
		t.Fatal("failed result publication retained identity lock")
	}
	retained, err := store.lookup(tenant, "key")
	if err != nil || retained.State != structuredRequestStateDispatched || len(retained.Result) != 0 {
		t.Fatalf("failed publication exposed result: state=%s error=%v", retained.State, err)
	}
	store.publish = publish
	if err := store.succeed(tenant, "key", intent, `{"decision":"pass"}`); err != nil {
		t.Fatal(err)
	}
	if len(store.identities.entries) != 0 {
		t.Fatal("replacement arrival retained identity lock")
	}
}
