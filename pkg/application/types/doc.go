// Package types holds the plain data types the public application surface
// shares with the internal packages that produce them. It stays free of
// non-stdlib imports by design: pkg/application imports internal/query and
// internal/model, so those packages can never import pkg/application back —
// this leaf package is the one cycle-free home for a type both sides name
// (s-tac-ah2). The exported-surface boundary test in pkg/application enforces
// that no exported signature names an internal type.
package types
