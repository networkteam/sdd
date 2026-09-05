package query

import "github.com/networkteam/sdd/pkg/application/types"

type SearchSyncMode = types.SearchSyncMode

const (
	SearchSyncNone  = types.SearchSyncNone
	SearchSyncLocal = types.SearchSyncLocal
	SearchSyncAll   = types.SearchSyncAll
)
