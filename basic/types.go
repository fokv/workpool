package basic

import "sync"

// Task should be a function receive nothing and return nothing
type Task func()

// Pool of work coroutines struct
type Pool struct {
	tasks   chan Task      //Task queue
	stop    chan struct{}  // close signal
	wg      sync.WaitGroup // Golang waiting group
	workers int            //Counts of works
}
