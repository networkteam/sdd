package types

type SearchDiscoveryCursor struct {
	Revision     string
	Namespace    IndexNamespace
	AfterEntryID string
}

type SearchEntryRequirement struct {
	Entry     SearchEntryDescriptor
	Published bool
	Cursor    SearchDiscoveryCursor
}
