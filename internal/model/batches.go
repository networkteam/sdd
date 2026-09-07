package model

import "iter"

// Batches consumes only enough input for the next batch. A source error follows
// any pending batch so its completed work can be retained by the caller.
func Batches[T any](source iter.Seq2[T, error], size int) iter.Seq2[[]T, error] {
	if size < 1 {
		panic("batch size must be positive")
	}
	return func(yield func([]T, error) bool) {
		batch := make([]T, 0, size)
		for item, err := range source {
			if err != nil {
				if len(batch) > 0 && !yield(batch, nil) {
					return
				}
				yield(nil, err)
				return
			}
			batch = append(batch, item)
			if len(batch) == size {
				if !yield(batch, nil) {
					return
				}
				batch = make([]T, 0, size)
			}
		}
		if len(batch) > 0 {
			yield(batch, nil)
		}
	}
}
