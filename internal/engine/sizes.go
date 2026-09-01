package engine

import (
	"encoding/json"
	"fmt"

	"github.com/networkteam/sdd/pkg/application/types"
)

// PartSize is defined in pkg/application/types — the exported surface names
// it, so the definition lives in the cycle-free public leaf (s-tac-ah2).
type PartSize = types.PartSize

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
