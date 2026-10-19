package proxy

import "sync"

// Digests identify the persisted tenant/key pair without an ambiguous join.
type structuredRequestIdentity struct {
	tenantDigest string
	keyDigest    string
}

type structuredIdentityLock struct {
	mutex      sync.Mutex
	references int
}

type structuredIdentityLocks struct {
	mutex   sync.Mutex
	entries map[structuredRequestIdentity]*structuredIdentityLock
}

// lock counts waiters before releasing the registry mutex. An entry cannot retire
// while another caller is acquiring it, so one identity always has one mutex.
func (locks *structuredIdentityLocks) lock(identity structuredRequestIdentity) func() {
	locks.mutex.Lock()
	if locks.entries == nil {
		locks.entries = make(map[structuredRequestIdentity]*structuredIdentityLock)
	}
	entry := locks.entries[identity]
	if entry == nil {
		entry = &structuredIdentityLock{}
		locks.entries[identity] = entry
	}
	entry.references++
	locks.mutex.Unlock()
	entry.mutex.Lock()
	return func() {
		entry.mutex.Unlock()
		locks.mutex.Lock()
		entry.references--
		if entry.references == 0 {
			delete(locks.entries, identity)
		}
		locks.mutex.Unlock()
	}
}

// lockRecord excludes whole-store recovery while allowing unrelated records to
// persist concurrently. Hosted callers enter with their database fence held.
func (store *structuredRequestStore) lockRecord(tenantDigest, keyDigest string) func() {
	store.mu.RLock()
	unlock := store.identities.lock(structuredRequestIdentity{tenantDigest: tenantDigest, keyDigest: keyDigest})
	return func() { unlock(); store.mu.RUnlock() }
}
