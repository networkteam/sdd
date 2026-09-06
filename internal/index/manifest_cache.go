package index

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
)

// ManifestCache reads one immutable publication file per identity change.
// Callers serialize access. Atomic rename makes file identity the freshness
// token even when a writer dies before updating the index generation marker.
type ManifestCache struct {
	identity fs.FileInfo
	manifest *Manifest
	loads    int
}

func (c *ManifestCache) Read(dir string) (_ *Manifest, err error) {
	file, err := os.Open(manifestPath(dir))
	if errors.Is(err, fs.ErrNotExist) {
		c.identity, c.manifest = nil, nil
		return &Manifest{Entries: map[string]EntryState{}}, nil
	}
	if err != nil {
		return nil, err
	}
	defer func() { err = errors.Join(err, file.Close()) }()
	identity, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if c.identity != nil && os.SameFile(c.identity, identity) && c.identity.ModTime().Equal(identity.ModTime()) && c.identity.Size() == identity.Size() {
		return c.manifest, nil
	}
	var manifest Manifest
	if err := json.NewDecoder(file).Decode(&manifest); err != nil {
		return nil, err
	}
	if manifest.Entries == nil {
		manifest.Entries = map[string]EntryState{}
	}
	c.identity, c.manifest = identity, &manifest
	c.loads++
	return c.manifest, nil
}

func (c *ManifestCache) Loads() int { return c.loads }
