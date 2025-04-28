package basic

import (
	"errors"
	"fmt"
	"sync"
)

// New goroutine pool with workers counts and capacity
func NewPool(workers, queueSize int) *Pool {
	p := &Pool{
		tasks:   make(chan Task, queueSize), //task queue capacity
		stop:    make(chan struct{}),
		wg:      sync.WaitGroup{},
		workers: workers, //tasks parallel executed counts
	}
	p.wg.Add(workers)

	for idx := 0; idx < workers; idx++ {
		go p.Work()
	}

	return p
}

// Worker goroutine
func (pool *Pool) Work() {
	defer pool.wg.Done()

	for {
		select {
		case task, ok := <-pool.tasks:
			if !ok {
				return
			}
			func() {
				defer func() {
					if r := recover(); r != nil {
						fmt.Printf("task panic recovered: %v\n", r)
					}
				}()
				task()
			}()
		}
	}
}

// Submit tasks to the pool
func (p *Pool) Submit(task Task) error {
	select {
	case p.tasks <- task:
		return nil
	case <-p.stop:
		return errors.New("Pool has been closed")
	}
}

// Gracefully close the pool
func (p *Pool) Close() {
	close(p.stop)
	close(p.tasks)
	p.wg.Wait()
}
