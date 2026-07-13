// Copyright 2026 Bitwise Media Group Ltd.
// SPDX-License-Identifier: MIT

package sourcectrl

import "sync"

// keyMutex hands out one mutex per accumulation key. Entries live for the
// process lifetime; the key space (repo × advisory families with recent
// activity) is small.
type keyMutex struct {
	mu    sync.Mutex
	locks map[string]*sync.Mutex
}

// lock acquires the key's mutex and returns its release func.
func (k *keyMutex) lock(key string) func() {
	k.mu.Lock()
	if k.locks == nil {
		k.locks = make(map[string]*sync.Mutex)
	}
	l, ok := k.locks[key]
	if !ok {
		l = new(sync.Mutex)
		k.locks[key] = l
	}
	k.mu.Unlock()

	l.Lock()
	return l.Unlock
}
