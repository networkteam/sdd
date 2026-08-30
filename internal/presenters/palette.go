package presenters

import "github.com/networkteam/sdd/internal/styles"

// Local names for the shared palette (internal/styles). Every styled (TTY) sdd
// presenter renders through these so human-facing output stays consistent with
// the command-output coordinator, which draws from the same source.
var (
	clrHeading  = styles.Heading
	clrIdentity = styles.Identity
	clrID       = styles.ID
	clrKey      = styles.Key
	clrRefKind  = styles.RefKind
	clrBody     = styles.Body
	clrQual     = styles.Qualifier
	clrFaint    = styles.Faint
	clrInactive = styles.Inactive
	clrWarn     = styles.Warn
)
