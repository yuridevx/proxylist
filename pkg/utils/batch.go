package utils

import (
	"context"
	"time"
)

func BatchSingle[T any](ctx context.Context, from chan T, to chan []T, flush time.Duration, count int) {
	buff := make([]T, count)
	idx := 0
	tmr := time.NewTimer(flush)
	defer tmr.Stop()
	defer close(to) // Close the 'to' channel when function returns

	for {
		select {
		case item, ok := <-from:
			if !ok {
				if idx != 0 {
					to <- buff[:idx]
				}
				return
			}
			tmr.Reset(flush)
			buff[idx] = item
			idx++
			if idx == count {
				to <- buff
				buff = make([]T, count)
				idx = 0
			}
		case <-tmr.C:
			if idx != 0 {
				to <- buff[:idx]
				buff = make([]T, count)
				idx = 0
			}
		case <-ctx.Done():
			return
		}
	}
}

func BatchAs[T any](ctx context.Context, from chan []T, to chan []T, flush time.Duration, count int) {
	buff := make([]T, count)
	idx := 0
	tmr := time.NewTimer(flush)
	defer tmr.Stop()
	defer close(to) // Close the 'to' channel when function returns

	for {
		select {
		case items, ok := <-from:
			if !ok {
				if idx != 0 {
					to <- buff[:idx]
				}
				return
			}
			tmr.Reset(flush)
			for i := 0; i < len(items); i++ {
				buff[idx] = items[i]
				idx++
				if idx == count {
					to <- buff
					buff = make([]T, count)
					idx = 0
				}
			}
		case <-tmr.C:
			if idx != 0 {
				to <- buff[:idx]
				buff = make([]T, count)
				idx = 0
			}
		case <-ctx.Done():
			return
		}
	}
}
