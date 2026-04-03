package daemon

import (
	"fmt"
	"sync"
	"sync/atomic"
)

// TaskLimiter controls concurrent task execution per-role.
// Uses atomic counters (AgentField pattern) — no dependency on container names.
type TaskLimiter struct {
	counts sync.Map // role → *int64
	max    map[string]int
}

func newTaskLimiter() *TaskLimiter {
	return &TaskLimiter{
		max: map[string]int{
			"leader": 1,
			"member": 3,
			"ops":    2,
		},
	}
}

func (l *TaskLimiter) Acquire(role string) error {
	maxVal := l.max[role]
	if maxVal == 0 {
		maxVal = 2 // default
	}
	actual, _ := l.counts.LoadOrStore(role, new(int64))
	counter := actual.(*int64)
	current := atomic.AddInt64(counter, 1)
	if current > int64(maxVal) {
		atomic.AddInt64(counter, -1)
		return fmt.Errorf("role %q has reached max concurrent tasks (%d)", role, maxVal)
	}
	return nil
}

func (l *TaskLimiter) Release(role string) {
	actual, ok := l.counts.Load(role)
	if !ok {
		return
	}
	counter := actual.(*int64)
	newVal := atomic.AddInt64(counter, -1)
	if newVal < 0 {
		atomic.StoreInt64(counter, 0)
	}
}
