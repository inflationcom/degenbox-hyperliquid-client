package relay

import (
	"sync"
	"time"
)

type InstructionDedup struct {
	mu      sync.Mutex
	seen    map[string]time.Time
	maxSize int
	ttl     time.Duration
}

func NewInstructionDedup(maxSize int, ttl time.Duration) *InstructionDedup {
	return &InstructionDedup{
		seen:    make(map[string]time.Time),
		maxSize: maxSize,
		ttl:     ttl,
	}
}

func (d *InstructionDedup) Check(id string) bool {
	d.mu.Lock()
	defer d.mu.Unlock()

	now := time.Now()

	if seen, ok := d.seen[id]; ok {
		if now.Sub(seen) < d.ttl {
			return false // duplicate
		}
	}

	if len(d.seen) >= d.maxSize {
		for k, t := range d.seen {
			if now.Sub(t) >= d.ttl {
				delete(d.seen, k)
			}
		}
	}

	// Still full after eviction — drop the oldest entry.
	if len(d.seen) >= d.maxSize {
		var oldest string
		var oldestTime time.Time
		for k, t := range d.seen {
			if oldest == "" || t.Before(oldestTime) {
				oldest = k
				oldestTime = t
			}
		}
		delete(d.seen, oldest)
	}

	d.seen[id] = now
	return true
}
