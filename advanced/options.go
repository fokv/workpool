package advanced

import "time"

// Option configures the WorkerPool
type Option func(*WorkerPool)

// WithMinWorkers sets the minimum number of workers
func WithMinWorkers(n int) Option {
	return func(p *WorkerPool) {
		p.configMu.Lock()
		defer p.configMu.Unlock()
		p.minWorkers = int32(n)
	}
}

// WithMaxWorkers sets the maximum number of workers
func WithMaxWorkers(n int) Option {
	return func(p *WorkerPool) {
		p.configMu.Lock()
		defer p.configMu.Unlock()
		p.maxWorkers = int32(n)
	}
}

// WithTaskTimeout sets the default task timeout
func WithTaskTimeout(timeout time.Duration) Option {
	return func(p *WorkerPool) {
		p.configMu.Lock()
		defer p.configMu.Unlock()
		p.taskTimeout = timeout
	}
}

// WithQueueSize sets the queue capacity
func WithQueueSize(size int) Option {
	return func(p *WorkerPool) {
		p.configMu.Lock()
		defer p.configMu.Unlock()
		p.queueSize = size
		for _, q := range p.queues {
			q.capacity = size
		}
	}
}

// WithPriorityPercentages sets worker allocation percentages
func WithPriorityPercentages(high, medium int) Option {
	return func(p *WorkerPool) {
		p.configMu.Lock()
		defer p.configMu.Unlock()
		p.highPriorityPct = high
		p.mediumPriorityPct = medium
	}
}
