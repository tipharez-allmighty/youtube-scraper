// Package channel.
package channel

import (
	"context"
	"sync"
)

func CloseWhenDone[T any](wg *sync.WaitGroup, ch chan T) {
	wg.Wait()
	close(ch)
}

func TryChannel[T any](ctx context.Context, ch chan<- T, v T) error {
	select {
	case ch <- v:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
