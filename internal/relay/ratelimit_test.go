package relay

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRateLimiter_UnderLimit(t *testing.T) {
	rl := newRateLimiter(5)
	for i := 0; i < 5; i++ {
		assert.True(t, rl.Allow(), "request %d should pass", i)
	}
}

func TestRateLimiter_AtLimit(t *testing.T) {
	rl := newRateLimiter(3)
	assert.True(t, rl.Allow())
	assert.True(t, rl.Allow())
	assert.True(t, rl.Allow())
	assert.False(t, rl.Allow(), "4th request should be rejected")
	assert.False(t, rl.Allow(), "5th request should also be rejected")
}

func TestRateLimiter_DefaultLimit(t *testing.T) {
	rl := newRateLimiter(0) // should default to 30
	for i := 0; i < 30; i++ {
		assert.True(t, rl.Allow())
	}
	assert.False(t, rl.Allow())
}

func TestRateLimiter_ConcurrentSafety(t *testing.T) {
	rl := newRateLimiter(100)
	var wg sync.WaitGroup
	for i := 0; i < 200; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			rl.Allow()
		}()
	}
	wg.Wait()
}
