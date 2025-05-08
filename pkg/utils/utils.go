package utils

func Discard[T any](ch chan T) {
	for {
		_, ok := <-ch
		if !ok {
			return
		}
	}
}

func Batch[T any](what []T, size int) [][]T {
	var batches [][]T
	for i := 0; i < len(what); i += size {
		end := i + size
		if end > len(what) {
			end = len(what)
		}
		batches = append(batches, what[i:end])
	}
	return batches
}
