package main

import "testing"

func TestDrainFuncsRunsQueuedFuncsInOrderThenStops(t *testing.T) {
	ch := make(chan func(), 4)
	var order []int
	ch <- func() { order = append(order, 1) }
	ch <- func() { order = append(order, 2) }
	ch <- func() { order = append(order, 3) }

	drainFuncs(ch)

	if len(order) != 3 || order[0] != 1 || order[1] != 2 || order[2] != 3 {
		t.Errorf("order = %v, want [1 2 3]", order)
	}
	select {
	case <-ch:
		t.Error("channel should be empty after draining")
	default:
	}

	// Draining an already-empty channel must not block.
	drainFuncs(ch)
}
