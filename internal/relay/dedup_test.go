package relay

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestInstructionDedup_FirstCheckPasses(t *testing.T) {
	d := NewInstructionDedup(100, time.Minute)
	assert.True(t, d.Check("abc"))
}

func TestInstructionDedup_DuplicateRejected(t *testing.T) {
	d := NewInstructionDedup(100, time.Minute)
	assert.True(t, d.Check("abc"))
	assert.False(t, d.Check("abc"))
}

func TestInstructionDedup_DifferentIDsPass(t *testing.T) {
	d := NewInstructionDedup(100, time.Minute)
	assert.True(t, d.Check("a"))
	assert.True(t, d.Check("b"))
	assert.True(t, d.Check("c"))
}

func TestInstructionDedup_TTLExpiry(t *testing.T) {
	d := NewInstructionDedup(100, 10*time.Millisecond)
	assert.True(t, d.Check("abc"))
	assert.False(t, d.Check("abc"))
	time.Sleep(15 * time.Millisecond)
	assert.True(t, d.Check("abc")) // TTL expired, should pass
}

func TestInstructionDedup_EvictionWhenFull(t *testing.T) {
	d := NewInstructionDedup(3, time.Minute)
	assert.True(t, d.Check("a"))
	assert.True(t, d.Check("b"))
	assert.True(t, d.Check("c"))
	// Full — next insert evicts oldest
	assert.True(t, d.Check("d"))
	// "a" was evicted
	assert.True(t, d.Check("a"))
}

func TestInstructionDedup_ConcurrentSafety(t *testing.T) {
	d := NewInstructionDedup(1000, time.Minute)
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			d.Check(fmt.Sprintf("id-%d", id))
		}(i)
	}
	wg.Wait()
}
