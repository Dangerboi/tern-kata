package worker

import "sync"

// keyedMutex hands out one mutex per key. The zero value is ready.
type keyedMutex struct {
	mu sync.Mutex
	m  map[string]*sync.Mutex
}

// get is the key's mutex, created on first use.
func (k *keyedMutex) get(key string) *sync.Mutex {
	k.mu.Lock()
	defer k.mu.Unlock()
	if k.m == nil {
		k.m = map[string]*sync.Mutex{}
	}
	if k.m[key] == nil {
		k.m[key] = new(sync.Mutex)
	}
	return k.m[key]
}

// retire forgets the key's mutex. Call it while holding that mutex, for a key that will not be used again.
func (k *keyedMutex) retire(key string) {
	k.mu.Lock()
	delete(k.m, key)
	k.mu.Unlock()
}
