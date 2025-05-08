package utils

func Discard[T any](ch chan T) {
	for {
		_, ok := <-ch
		if !ok {
			return
		}
	}
}

func Batch(what []string, size int) [][]string {
	var batches [][]string
	for i := 0; i < len(what); i += size {
		end := i + size
		if end > len(what) {
			end = len(what)
		}
		batches = append(batches, what[i:end])
	}
	return batches
}
