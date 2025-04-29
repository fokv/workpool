package advanced

import (
	"context"
	"sync"
	"sync/atomic"
	"time"
)

// Task represents a unit of work with priority
type Task struct {
	Priority    int               // Task priority (High/Medium/Low)
	Job         func() error      // The work function
	SubmittedAt time.Time         // When task was submitted
	Timeout     time.Duration     // Timeout for this task
	ResultChan  chan<- TaskResult // Channel for results
}

// TaskResult contains task execution results
type TaskResult struct {
	Err     error
	Runtime time.Duration
}

// PoolStats contains runtime statistics
type PoolStats struct {
	TotalWorkers   int32
	ActiveWorkers  int32
	HighPriority   int32
	MediumPriority int32
	QueueLengths   map[int]int
	Throughput     float64
	AvgTaskTime    time.Duration
	RejectedTasks  int32
	TimeoutTasks   int32
	HungerEvents   int32
}

// WorkerPool manages workers and task processing
type WorkerPool struct {
	// Configuration
	configMu          sync.RWMutex
	minWorkers        int32
	maxWorkers        int32
	taskTimeout       time.Duration
	monitorInterval   time.Duration
	hungerThreshold   time.Duration
	queueSize         int
	highPriorityPct   int
	mediumPriorityPct int

	// State
	stats              PoolStats
	lastThroughputCalc time.Time
	taskCount          int32
	totalRuntime       time.Duration

	// Queues
	queues     map[int]*priorityQueue
	queuesMu   sync.RWMutex
	queueNotif chan struct{}

	// Workers
	workersWg   sync.WaitGroup
	workerSlots chan struct{}
	taskChan    chan *Task
	workerStop  chan struct{}

	// Control
	ctx           context.Context
	cancel        context.CancelFunc
	closed        atomic.Bool
	shutdownOnce  sync.Once
	monitorTicker *time.Ticker
	hungerDetect  chan struct{}
}
