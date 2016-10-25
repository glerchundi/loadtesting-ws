package util

import (
	"errors"
	"sync"
	"time"
)

var (
	ErrTimeout = errors.New("timeout")
)

type WaitGroup struct {
	*sync.WaitGroup
}

func NewWaitGroup() *WaitGroup {
	return &WaitGroup{&sync.WaitGroup{}}
}

// WaitTimeout waits for the waitgroup for the specified max timeout.
func (wg *WaitGroup) WaitTimeout(timeout time.Duration) error {
	c := make(chan struct{})
	go func() {
		defer close(c)
		wg.Wait()
	}()

	select {
	case <-c:
		return nil
	case <-time.After(timeout):
		return ErrTimeout
	}
}
