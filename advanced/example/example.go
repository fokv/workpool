package main

import (
	"fmt"
	"time"

	workpool "github.com/fokv/workpool/advanced"
)

func main() {
	// Create a worker pool with:
	// - 5 minimum workers
	// - 20 maximum workers
	// - 20% high-priority workers
	// - 30% medium-priority workers
	pool := workpool.New(
		workpool.WithMinWorkers(5),
		workpool.WithMaxWorkers(20),
		workpool.WithPriorityPercentages(20, 30),
		workpool.WithTaskTimeout(10*time.Second),
	)
	defer pool.Shutdown()

	// Submit tasks with different priorities
	for i := 0; i < 100; i++ {
		priority := i % 3 // Rotate through priorities
		taskID := i

		err := pool.Submit(&workpool.Task{
			Priority: priority,
			Job: func() error {
				fmt.Printf("Processing task %d (priority %d)\n", taskID, priority)
				time.Sleep(time.Duration(100+(i*10)) * time.Millisecond)
				return nil
			},
		})

		if err != nil {
			fmt.Printf("Failed to submit task %d: %v\n", taskID, err)
		}
	}

	// Monitor pool stats
	for i := 0; i < 10; i++ {
		stats := pool.Stats()
		fmt.Printf("\nPool Stats (iteration %d):\n", i)
		fmt.Printf("Workers: %d/%d active\n", stats.ActiveWorkers, stats.TotalWorkers)
		fmt.Printf("Queue lengths - High: %d, Medium: %d, Low: %d\n",
			stats.QueueLengths[workpool.HighPriority],
			stats.QueueLengths[workpool.MediumPriority],
			stats.QueueLengths[workpool.LowPriority])
		fmt.Printf("Throughput: %.2f tasks/sec\n", stats.Throughput)
		fmt.Printf("Avg task time: %v\n", stats.AvgTaskTime)

		time.Sleep(1 * time.Second)
	}
}
