package main

import (
	"fmt"
	"github.com/fokv/workpool/basic"
	"time"
)

func main() {
	//Create pool include 3 works with capacity of 20
	pool := basic.NewPool(32, 20)

	//submit task
	for i := 0; i < 100; i++ {
		i := i
		err := pool.Submit(func() {
			time.Sleep(time.Second)
			fmt.Printf("Task %d executed by worker\n", i)
		})
		if err != nil {
			fmt.Println("submit error:", err)
		}
	}

	//submit a panic task
	pool.Submit(func() {
		panic("intentional panic")
	})

	//Waithing for all tasks done
	time.Sleep(50 * time.Second)

	//Close pool
	pool.Close()

	// Try to submit new task(should be error)
	err := pool.Submit(func() {})
	fmt.Println("Submit after close:", err)

}
