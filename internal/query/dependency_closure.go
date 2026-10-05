package query

// DependencyClosureQuery captures intent to list the repos the local repo
// depends on transitively through declared dependencies, in walk order
// (model.DependencyClosure).
type DependencyClosureQuery struct{}
