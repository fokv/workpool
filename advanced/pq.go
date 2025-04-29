package advanced

// priorityQueue implements heap.Interface
type priorityQueue struct {
	tasks    []*Task
	capacity int
}

// Len returns the number of tasks in the queue
func (pq *priorityQueue) Len() int {
	return len(pq.tasks)
}

// Less compares the priority of two tasks at positions i and j
func (pq *priorityQueue) Less(i, j int) bool {
	// We want Pop to give us the highest priority (not lowest),
	// so we use greater than here.
	return pq.tasks[i].Priority > pq.tasks[j].Priority
}

// Swap swaps the positions of two tasks in the queue
func (pq *priorityQueue) Swap(i, j int) {
	pq.tasks[i], pq.tasks[j] = pq.tasks[j], pq.tasks[i]
}

// Push adds a task to the queue
func (pq *priorityQueue) Push(x interface{}) {
	if pq.capacity > 0 && len(pq.tasks) >= pq.capacity {
		// Queue is at capacity, cannot add more
		return
	}
	task := x.(*Task)
	pq.tasks = append(pq.tasks, task)
}

// Pop removes and returns the highest priority task from the queue
func (pq *priorityQueue) Pop() interface{} {
	old := pq.tasks
	n := len(old)
	if n == 0 {
		return nil
	}
	task := old[n-1]
	pq.tasks = old[0 : n-1]
	return task
}

// Peek returns the highest priority task without removing it
func (pq *priorityQueue) Peek() *Task {
	if len(pq.tasks) == 0 {
		return nil
	}
	return pq.tasks[0]
}
