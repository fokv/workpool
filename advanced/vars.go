package advanced

import (
	"errors"
	"time"
)

// Priority levels
const (
	HighPriority   = 2
	MediumPriority = 1
	LowPriority    = 0
)

// Default configuration values
const (
	defaultMinWorkers      = 1
	defaultMaxWorkers      = 20
	defaultTaskTimeout     = 30 * time.Second
	defaultMonitorInterval = 5 * time.Second
	defaultHungerThreshold = 10 * time.Second
	defaultQueueSize       = 1000
)

// Error definitions
var (
	ErrPoolClosed      = errors.New("worker pool is closed")
	ErrTaskTimeout     = errors.New("task timed out")
	ErrQueueFull       = errors.New("task queue is full")
	ErrInvalidPriority = errors.New("invalid task priority")
)
