// Copyright 2026 Bitwise Media Group Ltd.
// SPDX-License-Identifier: MIT

package webhook

import "sync"

// dedup remembers the last cap delivery IDs, evicting oldest-first. GitHub
// redelivers on timeouts and manual replays; a bounded window is enough
// because handlers are idempotent anyway.
type dedup struct {
	mu    sync.Mutex
	seen  map[string]struct{}
	order []string
	next  int
}

func newDedup(cap int) *dedup {
	return &dedup{
		seen:  make(map[string]struct{}, cap),
		order: make([]string, cap),
	}
}

// add records the ID, reporting false when it was already present.
func (d *dedup) add(id string) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	if _, dup := d.seen[id]; dup {
		return false
	}
	if evict := d.order[d.next]; evict != "" {
		delete(d.seen, evict)
	}
	d.order[d.next] = id
	d.next = (d.next + 1) % len(d.order)
	d.seen[id] = struct{}{}
	return true
}

// remove forgets the ID (used when an accepted delivery could not be queued,
// so its redelivery must not look like a duplicate).
func (d *dedup) remove(id string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	delete(d.seen, id)
}
