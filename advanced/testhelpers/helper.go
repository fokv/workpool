package testhelpers

import (
	"time"

	workpool "github.com/fokv/workpool/advanced"
)

// BusyTask creates a task that runs for specified duration
func BusyTask(duration time.Duration) *workpool.Task {
	return &workpool.Task{
		Job: func() error {
			time.Sleep(duration)
			return nil
		},
	}
}

// BlockingTask creates a task that blocks until release channel is closed
func BlockingTask() (*workpool.Task, chan struct{}) {
	release := make(chan struct{})
	return &workpool.Task{
		Job: func() error {
			<-release
			return nil
		},
	}, release
}

// FailingTask creates a task that fails with given error
func FailingTask(err error) *workpool.Task {
	return &workpool.Task{
		Job: func() error {
			return err
		},
	}
}
