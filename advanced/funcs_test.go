package advanced

import (
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"sync"
	"testing"
	"time"
)

const testTimeout = 5 * time.Second

func TestNew(t *testing.T) {
	t.Parallel()

	t.Run("default configuration", func(t *testing.T) {
		pool := New()
		defer pool.Shutdown()

		stats := pool.Stats()
		assert.Equal(t, int32(1), stats.TotalWorkers) // minWorkers
		assert.Equal(t, int32(20), pool.maxWorkers)
	})

	t.Run("custom configuration", func(t *testing.T) {
		pool := New(
			WithMinWorkers(3),
			WithMaxWorkers(10),
			WithQueueSize(500),
			WithPriorityPercentages(30, 40),
		)
		defer pool.Shutdown()

		stats := pool.Stats()
		assert.Equal(t, int32(3), stats.TotalWorkers)
		assert.Equal(t, 10, int(pool.maxWorkers))
		assert.Equal(t, 500, pool.queueSize)
		assert.Equal(t, 30, pool.highPriorityPct)
		assert.Equal(t, 40, pool.mediumPriorityPct)
	})

	t.Run("invalid configuration panics", func(t *testing.T) {
		assert.Panics(t, func() {
			New(WithMinWorkers(-1))
		})
		assert.Panics(t, func() {
			New(WithPriorityPercentages(60, 50))
		})
	})
}

func TestSubmit(t *testing.T) {
	t.Parallel()

	t.Run("successful submission", func(t *testing.T) {
		pool := New(WithMinWorkers(1))
		defer pool.Shutdown()

		err := pool.Submit(&Task{
			Priority: HighPriority,
			Job:      func() error { return nil },
		})
		assert.NoError(t, err)
	})

	t.Run("invalid priority", func(t *testing.T) {
		pool := New()
		defer pool.Shutdown()

		err := pool.Submit(&Task{
			Priority: 3, // Invalid
			Job:      func() error { return nil },
		})
		assert.ErrorIs(t, err, ErrInvalidPriority)
	})

	t.Run("queue full", func(t *testing.T) {
		pool := New(WithQueueSize(1), WithMinWorkers(0))
		defer pool.Shutdown()

		// Fill the queue
		err := pool.Submit(&Task{
			Priority: HighPriority,
			Job:      func() error { time.Sleep(100 * time.Millisecond); return nil },
		})
		require.NoError(t, err)

		// Should fail
		err = pool.Submit(&Task{
			Priority: HighPriority,
			Job:      func() error { return nil },
		})
		assert.ErrorIs(t, err, ErrQueueFull)
	})

	t.Run("after shutdown", func(t *testing.T) {
		pool := New()
		pool.Shutdown()

		err := pool.Submit(&Task{
			Priority: HighPriority,
			Job:      func() error { return nil },
		})
		assert.ErrorIs(t, err, ErrPoolClosed)
	})
}

func TestPriorityExecution(t *testing.T) {
	t.Parallel()

	t.Run("priority ordering", func(t *testing.T) {
		pool := New(WithMinWorkers(1))
		defer pool.Shutdown()

		var executionOrder []int
		var mu sync.Mutex

		// Submit tasks in reverse priority order
		for i := 0; i < 3; i++ {
			priority := 2 - i // 2, 1, 0
			err := pool.Submit(&Task{
				Priority: priority,
				Job: func() error {
					mu.Lock()
					executionOrder = append(executionOrder, priority)
					mu.Unlock()
					return nil
				},
			})
			require.NoError(t, err)
		}

		assert.Eventually(t, func() bool {
			stats := pool.Stats()
			return stats.QueueLengths[HighPriority] == 0 &&
				stats.QueueLengths[MediumPriority] == 0 &&
				stats.QueueLengths[LowPriority] == 0
		}, testTimeout, 10*time.Millisecond)

		assert.Equal(t, []int{2, 1, 0}, executionOrder)
	})

	t.Run("priority worker allocation", func(t *testing.T) {
		pool := New(
			WithMinWorkers(3),
			WithPriorityPercentages(33, 33), // 1 high, 1 medium, 1 low
		)
		defer pool.Shutdown()

		// Block high priority worker
		highBlocker := make(chan struct{})
		pool.Submit(&Task{
			Priority: HighPriority,
			Job:      func() error { <-highBlocker; return nil },
		})

		// Block medium priority worker
		mediumBlocker := make(chan struct{})
		pool.Submit(&Task{
			Priority: MediumPriority,
			Job:      func() error { <-mediumBlocker; return nil },
		})

		// Submit tasks to test worker allocation
		var highRan, mediumRan, lowRan bool
		var wg sync.WaitGroup
		wg.Add(3)

		pool.Submit(&Task{
			Priority: HighPriority,
			Job:      func() error { highRan = true; wg.Done(); return nil },
		})
		pool.Submit(&Task{
			Priority: MediumPriority,
			Job:      func() error { mediumRan = true; wg.Done(); return nil },
		})
		pool.Submit(&Task{
			Priority: LowPriority,
			Job:      func() error { lowRan = true; wg.Done(); return nil },
		})

		// Release blockers
		close(highBlocker)
		close(mediumBlocker)

		wg.Wait()
		assert.True(t, highRan)
		assert.True(t, mediumRan)
		assert.True(t, lowRan)
	})
}

func TestDynamicScaling(t *testing.T) {
	t.Parallel()

	t.Run("manual resize", func(t *testing.T) {
		pool := New(WithMinWorkers(1), WithMaxWorkers(10))
		defer pool.Shutdown()

		pool.Resize(5)
		assert.Eventually(t, func() bool {
			return pool.Stats().TotalWorkers == 5
		}, testTimeout, 10*time.Millisecond)

		pool.Resize(2)
		assert.Eventually(t, func() bool {
			return pool.Stats().TotalWorkers == 2
		}, testTimeout, 10*time.Millisecond)
	})

	t.Run("automatic scaling under load", func(t *testing.T) {
		pool := New(WithMinWorkers(1), WithMaxWorkers(5))
		defer pool.Shutdown()

		// Submit tasks to create load
		var wg sync.WaitGroup
		for i := 0; i < 10; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				pool.Submit(&Task{
					Priority: LowPriority,
					Job:      func() error { time.Sleep(100 * time.Millisecond); return nil },
				})
			}()
		}
		wg.Wait()

		assert.Eventually(t, func() bool {
			return pool.Stats().TotalWorkers > 1
		}, testTimeout, 10*time.Millisecond)
	})
}

func TestTaskTimeout(t *testing.T) {
	t.Parallel()

	t.Run("global timeout", func(t *testing.T) {
		pool := New(WithMinWorkers(1), WithTaskTimeout(50*time.Millisecond))
		defer pool.Shutdown()

		resultChan := make(chan TaskResult, 1)
		err := pool.Submit(&Task{
			Job: func() error {
				time.Sleep(200 * time.Millisecond)
				return nil
			},
			ResultChan: resultChan,
		})
		require.NoError(t, err)

		result := <-resultChan
		assert.ErrorIs(t, result.Err, ErrTaskTimeout)
		assert.Less(t, result.Runtime, 100*time.Millisecond)
	})

	t.Run("per-task timeout", func(t *testing.T) {
		pool := New(WithMinWorkers(1))
		defer pool.Shutdown()

		resultChan := make(chan TaskResult, 1)
		err := pool.Submit(&Task{
			Job: func() error {
				time.Sleep(200 * time.Millisecond)
				return nil
			},
			Timeout:    50 * time.Millisecond,
			ResultChan: resultChan,
		})
		require.NoError(t, err)

		result := <-resultChan
		assert.ErrorIs(t, result.Err, ErrTaskTimeout)
	})
}

func TestHungerHandling(t *testing.T) {
	t.Parallel()

	t.Run("starvation detection", func(t *testing.T) {
		pool := New(
			WithMinWorkers(2),
			WithPriorityPercentages(50, 0), // 1 high-priority worker
		)
		defer pool.Shutdown()

		// Block high-priority worker
		blocker := make(chan struct{})
		pool.Submit(&Task{
			Priority: HighPriority,
			Job:      func() error { <-blocker; return nil },
		})

		// Fill with low-priority tasks
		for i := 0; i < 5; i++ {
			pool.Submit(&Task{
				Priority: LowPriority,
				Job:      func() error { return nil },
			})
		}

		// Wait for hunger detection
		assert.Eventually(t, func() bool {
			return pool.Stats().HungerEvents > 0
		}, testTimeout, 10*time.Millisecond)

		// Verify a low-priority task was promoted
		stats := pool.Stats()
		assert.Less(t, stats.QueueLengths[LowPriority], 5)

		// Release the blocker
		close(blocker)
	})
}

func TestGracefulShutdown(t *testing.T) {
	t.Parallel()

	t.Run("complete pending tasks", func(t *testing.T) {
		pool := New(WithMinWorkers(1))

		var completed bool
		pool.Submit(&Task{
			Job: func() error {
				time.Sleep(100 * time.Millisecond)
				completed = true
				return nil
			},
		})

		// Shutdown while task is running
		time.Sleep(50 * time.Millisecond)
		pool.Shutdown()

		assert.True(t, completed)
	})

	t.Run("reject new tasks after shutdown", func(t *testing.T) {
		pool := New()
		pool.Shutdown()

		err := pool.Submit(&Task{
			Job: func() error { return nil },
		})
		assert.ErrorIs(t, err, ErrPoolClosed)
	})
}

func TestConcurrentUsage(t *testing.T) {
	t.Parallel()

	t.Run("high concurrency", func(t *testing.T) {
		pool := New(WithMinWorkers(10), WithMaxWorkers(20))
		defer pool.Shutdown()

		var wg sync.WaitGroup
		const tasks = 100
		results := make(chan int, tasks)

		for i := 0; i < tasks; i++ {
			wg.Add(1)
			go func(n int) {
				defer wg.Done()
				pool.Submit(&Task{
					Priority: n % 3,
					Job: func() error {
						results <- n
						return nil
					},
				})
			}(i)
		}

		wg.Wait()

		// Verify all tasks completed
		assert.Eventually(t, func() bool {
			stats := pool.Stats()
			return len(results) == tasks &&
				stats.QueueLengths[HighPriority] == 0 &&
				stats.QueueLengths[MediumPriority] == 0 &&
				stats.QueueLengths[LowPriority] == 0
		}, testTimeout, 10*time.Millisecond)
	})
}

func TestStats(t *testing.T) {
	t.Parallel()

	pool := New(WithMinWorkers(1), WithTaskTimeout(100*time.Millisecond))
	defer pool.Shutdown()

	// Submit some tasks
	for i := 0; i < 5; i++ {
		pool.Submit(&Task{
			Priority: HighPriority,
			Job:      func() error { time.Sleep(50 * time.Millisecond); return nil },
		})
	}

	// Wait for stats to update
	assert.Eventually(t, func() bool {
		return pool.Stats().Throughput > 0
	}, testTimeout, 10*time.Millisecond)

	stats := pool.Stats()
	assert.Greater(t, stats.Throughput, 0.0)
	assert.Greater(t, stats.AvgTaskTime, time.Duration(0))
}
