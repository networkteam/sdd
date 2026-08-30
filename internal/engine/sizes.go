package engine

import (
	"encoding/json"
	"fmt"
)

// PartSize names one serve part and its byte size — the per-part accounting
// behind the serve-budget measurement (d-tac-qwc). Injects and lanes are the
// scaling parts; schema, diagnostics, and produced complete a serve's weight.
type PartSize struct {
	Part  string
	Bytes int
}

// partBytes measures a part as it reaches the wire: strings by length,
// everything else by its JSON encoding.
func partBytes(v any) int {
	switch t := v.(type) {
	case nil:
		return 0
	case string:
		return len(t)
	default:
		b, err := json.Marshal(v)
		if err != nil {
			return len(fmt.Sprintf("%v", v))
		}
		return len(b)
	}
}
