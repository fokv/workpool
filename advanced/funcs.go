package advanced

import (
	"container/heap"
	"context"
	"errors"
	"sync/atomic"
	"time"
)

// New creates a new WorkerPool with default configuration
func New(options ...Option) *WorkerPool {
	ctx, cancel := context.WithCancel(context.Background())

	pool := &WorkerPool{
		minWorkers:        defaultMinWorkers,
		maxWorkers:        defaultMaxWorkers,
		taskTimeout:       defaultTaskTimeout,
		monitorInterval:   defaultMonitorInterval,
		hungerThreshold:   defaultHungerThreshold,
		queueSize:         defaultQueueSize,
		highPriorityPct:   20,
		mediumPriorityPct: 30,

		queues: map[int]*priorityQueue{
			HighPriority:   {capacity: defaultQueueSize},
			MediumPriority: {capacity: defaultQueueSize},
			LowPriority:    {capacity: defaultQueueSize},
		},
		queueNotif:    make(chan struct{}, 1),
		workerSlots:   make(chan struct{}, defaultMaxWorkers),
		taskChan:      make(chan *Task, defaultQueueSize),
		workerStop:    make(chan struct{}),
		ctx:           ctx,
		cancel:        cancel,
		monitorTicker: time.NewTicker(defaultMonitorInterval),
		hungerDetect:  make(chan struct{}, 1),
	}

	// Initialize queues
	for _, q := range pool.queues {
		heap.Init(q)
	}

	// Apply options
	for _, opt := range options {
		opt(pool)
	}

	// Initialize worker slots
	for i := 0; i < int(pool.minWorkers); i++ {
		pool.workerSlots <- struct{}{}
	}

	// Start workers
	for i := 0; i < int(pool.minWorkers); i++ {
		pool.startWorker()
	}

	// Start management goroutines
	go pool.dispatcher()
	go pool.monitor()
	go pool.hungerDetection()

	return pool
}

// Submit adds a task to the work pool
func (p *WorkerPool) Submit(task *Task) error {
	if p.closed.Load() {
		return ErrPoolClosed
	}

	if task.Priority < LowPriority || task.Priority > HighPriority {
		return ErrInvalidPriority
	}

	// Set defaults
	if task.Timeout == 0 {
		task.Timeout = p.taskTimeout
	}
	task.SubmittedAt = time.Now()

	// Add to appropriate queue
	p.queuesMu.Lock()
	queue := p.queues[task.Priority]
	if queue.capacity > 0 && len(queue.tasks) >= queue.capacity {
		p.queuesMu.Unlock()
		atomic.AddInt32(&p.stats.RejectedTasks, 1)
		return ErrQueueFull
	}
	heap.Push(queue, task)
	p.queuesMu.Unlock()

	// Notify dispatcher
	select {
	case p.queueNotif <- struct{}{}:
	default:
	}

	return nil
}

// SubmitWithContext adds a task with context support
func (p *WorkerPool) SubmitWithContext(ctx context.Context, task *Task) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
		return p.Submit(task)
	}
}

// Stats returns current pool statistics
func (p *WorkerPool) Stats() PoolStats {
	p.queuesMu.RLock()
	queueLengths := map[int]int{
		HighPriority:   p.queues[HighPriority].Len(),
		MediumPriority: p.queues[MediumPriority].Len(),
		LowPriority:    p.queues[LowPriority].Len(),
	}
	p.queuesMu.RUnlock()

	p.configMu.RLock()
	defer p.configMu.RUnlock()

	return PoolStats{
		TotalWorkers:   atomic.LoadInt32(&p.stats.TotalWorkers),
		ActiveWorkers:  atomic.LoadInt32(&p.stats.ActiveWorkers),
		HighPriority:   atomic.LoadInt32(&p.stats.HighPriority),
		MediumPriority: atomic.LoadInt32(&p.stats.MediumPriority),
		QueueLengths:   queueLengths,
		Throughput:     p.stats.Throughput,
		AvgTaskTime:    p.stats.AvgTaskTime,
		RejectedTasks:  atomic.LoadInt32(&p.stats.RejectedTasks),
		TimeoutTasks:   atomic.LoadInt32(&p.stats.TimeoutTasks),
		HungerEvents:   atomic.LoadInt32(&p.stats.HungerEvents),
	}
}

// Resize adjusts the worker pool size
func (p *WorkerPool) Resize(n int) {
	p.configMu.Lock()
	defer p.configMu.Unlock()

	if n < int(p.minWorkers) {
		n = int(p.minWorkers)
	} else if n > int(p.maxWorkers) {
		n = int(p.maxWorkers)
	}

	current := int(atomic.LoadInt32(&p.stats.TotalWorkers))
	if n == current {
		return
	}

	// Adjust worker slots
	if n > current {
		for i := 0; i < n-current; i++ {
			select {
			case p.workerSlots <- struct{}{}:
			default:
			}
		}
	} else {
		for i := 0; i < current-n; i++ {
			select {
			case p.workerStop <- struct{}{}:
			default:
			}
		}
	}
}

// Shutdown gracefully shuts down the pool
func (p *WorkerPool) Shutdown() {
	p.shutdownOnce.Do(func() {
		p.closed.Store(true)
		p.cancel()
		p.monitorTicker.Stop()
		close(p.workerStop)
		p.workersWg.Wait()
		close(p.taskChan)
	})
}

// Private methods

func (p *WorkerPool) startWorker() {
	select {
	case <-p.workerSlots:
		p.workersWg.Add(1)
		atomic.AddInt32(&p.stats.TotalWorkers, 1)
		go p.worker()
	default:
	}
}

func (p *WorkerPool) worker() {
	defer func() {
		p.workersWg.Done()
		atomic.AddInt32(&p.stats.TotalWorkers, -1)
		p.workerSlots <- struct{}{}
	}()

	for {
		select {
		case <-p.workerStop:
			return
		case task := <-p.taskChan:
			start := time.Now()

			// Track priority worker usage
			var priorityActive *int32
			if task.Priority == HighPriority {
				priorityActive = &p.stats.HighPriority
			} else if task.Priority == MediumPriority {
				priorityActive = &p.stats.MediumPriority
			}

			if priorityActive != nil {
				atomic.AddInt32(priorityActive, 1)
			}
			atomic.AddInt32(&p.stats.ActiveWorkers, 1)

			// Execute task
			err := p.executeTask(task)

			runtime := time.Since(start)
			atomic.AddInt32(&p.stats.ActiveWorkers, -1)
			if priorityActive != nil {
				atomic.AddInt32(priorityActive, -1)
			}
			atomic.AddInt32(&p.taskCount, 1)
			p.totalRuntime += runtime

			// Send result if requested
			if task.ResultChan != nil {
				task.ResultChan <- TaskResult{
					Err:     err,
					Runtime: runtime,
				}
			}
		}
	}
}

func (p *WorkerPool) executeTask(task *Task) error {
	ctx, cancel := context.WithTimeout(p.ctx, task.Timeout)
	defer cancel()

	done := make(chan error, 1)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				done <- errors.New("task panicked")
			}
		}()
		done <- task.Job()
	}()

	select {
	case <-ctx.Done():
		atomic.AddInt32(&p.stats.TimeoutTasks, 1)
		return ErrTaskTimeout
	case err := <-done:
		return err
	}
}

func (p *WorkerPool) dispatcher() {
	for {
		select {
		case <-p.ctx.Done():
			return
		case <-p.queueNotif:
			p.dispatchTasks()
		}
	}
}

func (p *WorkerPool) dispatchTasks() {
	p.queuesMu.Lock()
	defer p.queuesMu.Unlock()

	// Try to dispatch tasks in priority order
	for priority := HighPriority; priority >= LowPriority; priority-- {
		queue := p.queues[priority]
		for queue.Len() > 0 {
			task := queue.Peek()

			select {
			case p.taskChan <- task:
				heap.Pop(queue)
			default:
				return // Workers busy
			}
		}
	}
}

func (p *WorkerPool) monitor() {
	for {
		select {
		case <-p.ctx.Done():
			return
		case <-p.monitorTicker.C:
			p.adjustWorkers()
			p.calculateThroughput()

			// Check for potential starvation
			p.queuesMu.RLock()
			lowQueueLen := p.queues[LowPriority].Len()
			p.queuesMu.RUnlock()

			if lowQueueLen > 0 {
				select {
				case p.hungerDetect <- struct{}{}:
				default:
				}
			}
		}
	}
}

func (p *WorkerPool) hungerDetection() {
	hungerTimer := time.NewTimer(p.hungerThreshold)
	defer hungerTimer.Stop()

	for {
		select {
		case <-p.ctx.Done():
			return
		case <-p.hungerDetect:
			hungerTimer.Reset(p.hungerThreshold)
		case <-hungerTimer.C:
			p.handleHunger()
			hungerTimer.Reset(p.hungerThreshold)
		}
	}
}

func (p *WorkerPool) handleHunger() {
	p.queuesMu.Lock()
	defer p.queuesMu.Unlock()

	lowQueue := p.queues[LowPriority]
	if lowQueue.Len() == 0 {
		return
	}

	// Find oldest low-priority task
	var oldestIdx int
	oldestTime := time.Now()
	for i, task := range lowQueue.tasks {
		if task.SubmittedAt.Before(oldestTime) {
			oldestTime = task.SubmittedAt
			oldestIdx = i
		}
	}

	// Promote to medium priority
	task := lowQueue.tasks[oldestIdx]
	task.Priority = MediumPriority
	heap.Push(p.queues[MediumPriority], task)

	// Remove from low priority queue
	heap.Remove(lowQueue, oldestIdx)
	atomic.AddInt32(&p.stats.HungerEvents, 1)
}

func (p *WorkerPool) adjustWorkers() {
	p.configMu.RLock()
	maxWorkers := p.maxWorkers
	minWorkers := p.minWorkers
	p.configMu.RUnlock()

	current := atomic.LoadInt32(&p.stats.TotalWorkers)
	active := atomic.LoadInt32(&p.stats.ActiveWorkers)

	p.queuesMu.RLock()
	totalQueued := p.queues[HighPriority].Len() +
		p.queues[MediumPriority].Len() +
		p.queues[LowPriority].Len()
	p.queuesMu.RUnlock()

	// Scale up if needed
	if totalQueued > 0 && float64(active)/float64(current) > 0.7 {
		if current < maxWorkers {
			p.startWorker()
		}
		return
	}

	// Scale down if possible
	if totalQueued == 0 && current > minWorkers && float64(active)/float64(current) < 0.3 {
		select {
		case p.workerStop <- struct{}{}:
		default:
		}
	}
}

func (p *WorkerPool) calculateThroughput() {
	now := time.Now()
	if p.lastThroughputCalc.IsZero() {
		p.lastThroughputCalc = now
		return
	}

	elapsed := now.Sub(p.lastThroughputCalc).Seconds()
	taskCount := atomic.SwapInt32(&p.taskCount, 0)

	if elapsed > 0 {
		p.stats.Throughput = float64(taskCount) / elapsed
	}

	if taskCount > 0 {
		p.stats.AvgTaskTime = p.totalRuntime / time.Duration(taskCount)
		p.totalRuntime = 0
	}

	p.lastThroughputCalc = now
}
