package relay

import (
	"log/slog"
	"sync"
	"time"
)

// CircuitBreaker trips (pauses the client) after too many consecutive
// order failures in a rolling window.
type CircuitBreaker struct {
	mu               sync.Mutex
	maxConsecutive   int
	cooldown         time.Duration
	consecutiveFails int
	trippedAt        time.Time
	onTrip           func() // called when breaker trips
}

// NewCircuitBreaker creates a breaker that trips after maxConsecutive failures.
// Set maxConsecutive to 0 to disable.
func NewCircuitBreaker(maxConsecutive int, cooldown time.Duration, onTrip func()) *CircuitBreaker {
	if maxConsecutive <= 0 {
		maxConsecutive = 0 // disabled
	}
	if cooldown <= 0 {
		cooldown = 30 * time.Minute
	}
	return &CircuitBreaker{
		maxConsecutive: maxConsecutive,
		cooldown:       cooldown,
		onTrip:         onTrip,
	}
}

// RecordSuccess resets the consecutive failure counter.
func (cb *CircuitBreaker) RecordSuccess() {
	if cb.maxConsecutive == 0 {
		return
	}
	cb.mu.Lock()
	cb.consecutiveFails = 0
	cb.mu.Unlock()
}

// RecordFailure increments the failure counter and trips if threshold reached.
func (cb *CircuitBreaker) RecordFailure() {
	if cb.maxConsecutive == 0 {
		return
	}
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.consecutiveFails++
	if cb.consecutiveFails >= cb.maxConsecutive {
		if time.Since(cb.trippedAt) < cb.cooldown {
			return // already tripped recently
		}
		cb.trippedAt = time.Now()
		slog.Error("circuit breaker tripped",
			"consecutive_failures", cb.consecutiveFails,
			"threshold", cb.maxConsecutive,
		)
		cb.consecutiveFails = 0
		if cb.onTrip != nil {
			go cb.onTrip()
		}
	}
}
