package vectors

import (
	"fmt"
	"math"
)

func Validate(vectors [][]float32, count int) error {
	if len(vectors) != count {
		return fmt.Errorf("sdd: embedder returned %d vectors for %d chunks", len(vectors), count)
	}
	dims := 0
	for _, vector := range vectors {
		if len(vector) == 0 {
			return fmt.Errorf("sdd: empty vector")
		}
		if dims == 0 {
			dims = len(vector)
		}
		if len(vector) != dims {
			return fmt.Errorf("sdd: inconsistent vector dimensions")
		}
		norm := float64(0)
		for _, v := range vector {
			if math.IsNaN(float64(v)) || math.IsInf(float64(v), 0) {
				return fmt.Errorf("sdd: non-finite vector")
			}
			norm += float64(v) * float64(v)
		}
		if norm == 0 {
			return fmt.Errorf("sdd: zero vector")
		}
	}
	return nil
}
