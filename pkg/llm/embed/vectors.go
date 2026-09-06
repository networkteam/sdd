package embed

import (
	"fmt"
	"math"
)

func validateBatchVectors(vectors [][]float32, count int) error {
	if len(vectors) != count {
		return fmt.Errorf("embed: got %d vectors for %d texts", len(vectors), count)
	}
	dims := 0
	for _, vector := range vectors {
		if len(vector) == 0 {
			return fmt.Errorf("embed: empty vector")
		}
		if dims == 0 {
			dims = len(vector)
		}
		if len(vector) != dims {
			return fmt.Errorf("embed: inconsistent vector dimensions")
		}
		norm := float64(0)
		for _, v := range vector {
			if math.IsNaN(float64(v)) || math.IsInf(float64(v), 0) {
				return fmt.Errorf("embed: non-finite vector")
			}
			norm += float64(v) * float64(v)
		}
		if norm == 0 {
			return fmt.Errorf("embed: zero vector")
		}
	}
	return nil
}
