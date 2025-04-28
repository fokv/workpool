package basic

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestNewPool(t *testing.T) {
	workers := 5
	queueSize := 10
	pool := NewPool(workers, queueSize)

	assert.NotNil(t, pool)
	assert.Equal(t, workers, pool.workers)
	assert.Equal(t, queueSize, cap(pool.tasks))
}
