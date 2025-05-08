package utils

import (
	"context"
	"time"
)

func OnlyUnique(ctx context.Context, from chan []string, to chan []string, keepInMemory time.Duration) {
	uniqueMap := make(map[string]time.Time)
	defer close(to)

	cleanupTicker := time.NewTicker(keepInMemory)
	defer cleanupTicker.Stop()

	for {
		select {
		case strs, ok := <-from:
			if !ok {
				return
			}
			now := time.Now()
			uniqueBatch := make([]string, 0, len(strs))
			for _, str := range strs {
				if expTime, exists := uniqueMap[str]; !exists || now.After(expTime) {
					uniqueMap[str] = now.Add(keepInMemory)
					uniqueBatch = append(uniqueBatch, str)
				}
			}
			if len(uniqueBatch) > 0 {
				to <- uniqueBatch
			}
		case <-cleanupTicker.C:
			now := time.Now()
			for k, v := range uniqueMap {
				if now.After(v) {
					delete(uniqueMap, k)
				}
			}
		case <-ctx.Done():
			return
		}
	}
}
