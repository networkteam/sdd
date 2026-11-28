package application_test

import (
	"github.com/networkteam/sdd/internal/presenters"
	sdd "github.com/networkteam/sdd/pkg/application"
)

// showText and searchText render a read as the MCP surface serves it.
func showText(r sdd.ShowResult) string     { return presenters.ShowText(r.Entries) }
func searchText(r sdd.SearchResult) string { return presenters.SearchText(r.Hits) }
