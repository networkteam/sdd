package types

// ServeLane is one rendered lane of a serve's instruction unit — the unit a
// host's served-once memory dedups at (d-tac-87o).
type ServeLane struct {
	Name string
	Text string
}

// PartSize names one serve part and its byte size — the per-part accounting
// behind the serve-budget measurement (d-tac-qwc). Injects and lanes are the
// scaling parts; schema, diagnostics, and produced complete a serve's weight.
type PartSize struct {
	Part  string
	Bytes int
}
