package utils

import (
	"context"
	"time"
)

func BatchAs(ctx context.Context, from chan []string, to chan []string, flush time.Duration, count int) {
	buff := make([]string, count)
	idx := 0
	tmr := time.NewTimer(flush)
	defer tmr.Stop()
	defer close(to) // Close the 'to' channel when function returns

	for {
		select {
		case strs, ok := <-from:
			if !ok {
				if idx != 0 {
					to <- buff[:idx]
				}
				return
			}
			tmr.Reset(flush)
			for i := 0; i < len(strs); i++ {
				buff[idx] = strs[i]
				idx++
				if idx == count {
					to <- buff
					buff = make([]string, count)
					idx = 0
				}
			}
		case <-tmr.C:
			if idx != 0 {
				to <- buff[:idx]
				buff = make([]string, count)
				idx = 0
			}
		case <-ctx.Done():
			return
		}
	}
}
