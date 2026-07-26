package local

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

type trustedDirectoryComponent struct {
	parent   *os.Root
	root     *os.Root
	name     string
	display  string
	identity fs.FileInfo
}

// trustedDirectoryChain anchors a directory path at an already-open root and
// walks every component descriptor-relatively. Each opened component remains
// pinned so revalidation detects replacement of any configured ancestor.
type trustedDirectoryChain struct {
	base          *os.Root
	ownsBase      bool
	components    []trustedDirectoryComponent
	complete      bool
	missingParent *os.Root
	missingName   string
	display       string
}

func trustedVolumeRoot(path string) string {
	volume := filepath.VolumeName(path)
	if volume == "" {
		return string(filepath.Separator)
	}
	return volume + string(filepath.Separator)
}

func openTrustedAbsoluteDirectoryChain(
	path string,
	create bool,
	allowMissing bool,
	mode fs.FileMode,
) (*trustedDirectoryChain, error) {
	path = filepath.Clean(path)
	if !filepath.IsAbs(path) {
		return nil, fmt.Errorf("trusted directory path must be absolute: %q", path)
	}
	volumeRoot := trustedVolumeRoot(path)
	relative, err := filepath.Rel(volumeRoot, path)
	if err != nil || relative == "." || !filepath.IsLocal(relative) {
		return nil, fmt.Errorf("trusted directory must be below its filesystem volume root: %q", path)
	}
	root, err := os.OpenRoot(volumeRoot)
	if err != nil {
		return nil, fmt.Errorf("opening filesystem volume root %s: %w", volumeRoot, err)
	}
	chain, err := openTrustedRelativeDirectoryChain(
		root, relative, create, allowMissing, mode, path,
	)
	if err != nil {
		return nil, errors.Join(err, root.Close())
	}
	chain.ownsBase = true
	return chain, nil
}

func openTrustedRelativeDirectoryChain(
	base *os.Root,
	relative string,
	create bool,
	allowMissing bool,
	mode fs.FileMode,
	display string,
) (*trustedDirectoryChain, error) {
	relative = filepath.Clean(relative)
	if relative == "." || !filepath.IsLocal(relative) {
		return nil, fmt.Errorf("trusted directory path must contain local components: %q", relative)
	}
	chain := &trustedDirectoryChain{base: base, display: display}
	fail := func(cause error) (*trustedDirectoryChain, error) {
		return nil, errors.Join(cause, chain.close())
	}
	parent := base
	var prefix string
	parts := strings.Split(filepath.ToSlash(relative), "/")
	displayBase := filepath.Clean(display)
	for range parts {
		displayBase = filepath.Dir(displayBase)
	}
	for _, slashPart := range parts {
		part := filepath.FromSlash(slashPart)
		if part == "" || part == "." || part == ".." || filepath.Base(part) != part {
			return fail(fmt.Errorf("trusted directory contains invalid component %q", slashPart))
		}
		if prefix == "" {
			prefix = part
		} else {
			prefix = filepath.Join(prefix, part)
		}
		componentDisplay := filepath.Join(displayBase, prefix)
		info, err := parent.Lstat(part)
		if errors.Is(err, fs.ErrNotExist) && create {
			if mkdirErr := parent.Mkdir(part, mode); mkdirErr != nil && !errors.Is(mkdirErr, fs.ErrExist) {
				return fail(fmt.Errorf("creating trusted directory component %s: %w", componentDisplay, mkdirErr))
			}
			info, err = parent.Lstat(part)
		}
		if errors.Is(err, fs.ErrNotExist) && allowMissing {
			chain.missingParent = parent
			chain.missingName = part
			return chain, nil
		}
		if err != nil {
			return fail(fmt.Errorf("opening trusted directory component %s: %w", componentDisplay, err))
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fail(fmt.Errorf("trusted directory component %s is a symbolic link", componentDisplay))
		}
		if !info.IsDir() {
			return fail(fmt.Errorf("trusted directory component %s is not a directory", componentDisplay))
		}
		child, err := parent.OpenRoot(part)
		if err != nil {
			return fail(fmt.Errorf("opening trusted directory component %s: %w", componentDisplay, err))
		}
		identity, err := child.Stat(".")
		if err != nil {
			_ = child.Close()
			return fail(err)
		}
		atName, err := parent.Lstat(part)
		if err != nil || atName.Mode()&os.ModeSymlink != 0 || !atName.IsDir() ||
			!os.SameFile(identity, atName) {
			_ = child.Close()
			if err == nil {
				err = fmt.Errorf("trusted directory component was rebound while opening")
			}
			return fail(fmt.Errorf("opening trusted directory component %s: %w", componentDisplay, err))
		}
		chain.components = append(chain.components, trustedDirectoryComponent{
			parent: parent, root: child, name: part, display: componentDisplay, identity: identity,
		})
		parent = child
	}
	chain.complete = true
	return chain, nil
}

func (c *trustedDirectoryChain) root() *os.Root {
	if c == nil || !c.complete || len(c.components) == 0 {
		return nil
	}
	return c.components[len(c.components)-1].root
}

func (c *trustedDirectoryChain) revalidate() error {
	if c == nil {
		return fmt.Errorf("trusted directory chain is not open")
	}
	for _, component := range c.components {
		atName, err := component.parent.Lstat(component.name)
		if err != nil || atName.Mode()&os.ModeSymlink != 0 || !atName.IsDir() ||
			!os.SameFile(component.identity, atName) {
			return fmt.Errorf("trusted directory component was rebound: %s", component.display)
		}
	}
	if !c.complete {
		if _, err := c.missingParent.Lstat(c.missingName); !errors.Is(err, fs.ErrNotExist) {
			if err == nil {
				return fmt.Errorf("previously absent trusted directory appeared: %s", c.display)
			}
			return err
		}
	}
	return nil
}

func (c *trustedDirectoryChain) removeVerifiedLeaf() error {
	if c == nil || !c.complete || len(c.components) == 0 {
		return fmt.Errorf("trusted directory chain has no pinned leaf to remove")
	}
	if err := c.revalidate(); err != nil {
		return err
	}
	last := len(c.components) - 1
	leaf := &c.components[last]
	if err := leaf.root.Close(); err != nil {
		return err
	}
	leaf.root = nil
	atName, err := leaf.parent.Lstat(leaf.name)
	if err != nil || atName.Mode()&os.ModeSymlink != 0 || !atName.IsDir() ||
		!os.SameFile(leaf.identity, atName) {
		if err == nil {
			err = fmt.Errorf("trusted directory leaf was rebound before removal")
		}
		return fmt.Errorf("removing trusted directory leaf %s: %w", leaf.display, err)
	}
	if err := leaf.parent.Remove(leaf.name); err != nil {
		return fmt.Errorf("removing trusted directory leaf %s: %w", leaf.display, err)
	}
	if err := syncRootDirectory(leaf.parent, "."); err != nil {
		return err
	}
	c.components = c.components[:last]
	c.complete = false
	c.missingParent = leaf.parent
	c.missingName = leaf.name
	return nil
}

func (c *trustedDirectoryChain) close() error {
	if c == nil {
		return nil
	}
	var errs []error
	for index := len(c.components) - 1; index >= 0; index-- {
		if c.components[index].root != nil {
			errs = append(errs, c.components[index].root.Close())
			c.components[index].root = nil
		}
	}
	if c.ownsBase && c.base != nil {
		errs = append(errs, c.base.Close())
		c.base = nil
	}
	return errors.Join(errs...)
}

// trustedStateAuthority pins the complete configured state path from the
// filesystem volume root. Relocation categories share one instance.
type trustedStateAuthority struct {
	chain     *trustedDirectoryChain
	state     *os.Root
	statePath string
}

func openTrustedStateAuthority(statePath string, create bool) (*trustedStateAuthority, error) {
	chain, err := openTrustedAbsoluteDirectoryChain(statePath, create, false, 0o700)
	if err != nil {
		return nil, err
	}
	result := &trustedStateAuthority{
		chain: chain, state: chain.root(), statePath: filepath.Clean(statePath),
	}
	if err := result.revalidate(); err != nil {
		return nil, errors.Join(err, chain.close())
	}
	return result, nil
}

func (a *trustedStateAuthority) revalidate() error {
	if err := a.chain.revalidate(); err != nil {
		return fmt.Errorf("trusted state root was rebound for %s: %w", a.statePath, err)
	}
	return nil
}

func (a *trustedStateAuthority) close() error {
	if a == nil {
		return nil
	}
	return a.chain.close()
}

type trustedStoreRoot struct {
	authority *trustedStateAuthority
	category  *trustedDirectoryChain
	storeKey  *trustedDirectoryChain
	store     *os.Root
}

func openTrustedStoreRoot(
	statePath string,
	categoryName string,
	storePath string,
	create bool,
) (*trustedStoreRoot, error) {
	categoryPath := filepath.Join(statePath, categoryName)
	relative, err := filepath.Rel(categoryPath, storePath)
	if err != nil || !filepath.IsLocal(relative) {
		return nil, fmt.Errorf("%s store escapes trusted state category", categoryName)
	}
	authority, err := openTrustedStateAuthority(statePath, create)
	if err != nil {
		return nil, err
	}
	category, err := openTrustedRelativeDirectoryChain(
		authority.state, categoryName, create, false, 0o700, categoryPath,
	)
	if err != nil {
		return nil, errors.Join(err, authority.close())
	}
	var storeKey *trustedDirectoryChain
	store := category.root()
	if relative != "." {
		storeKey, err = openTrustedRelativeDirectoryChain(
			category.root(), relative, create, false, 0o755, storePath,
		)
		if err != nil {
			return nil, errors.Join(err, category.close(), authority.close())
		}
		store = storeKey.root()
	}
	result := &trustedStoreRoot{
		authority: authority, category: category, storeKey: storeKey, store: store,
	}
	if err := result.revalidate(); err != nil {
		return nil, errors.Join(err, storeKey.close(), category.close(), authority.close())
	}
	return result, nil
}

func (r *trustedStoreRoot) revalidate() error {
	if err := r.authority.revalidate(); err != nil {
		return err
	}
	if err := r.category.revalidate(); err != nil {
		return err
	}
	if r.storeKey != nil {
		if err := r.storeKey.revalidate(); err != nil {
			return err
		}
	}
	return nil
}

func (r *trustedStoreRoot) close() error {
	if r == nil {
		return nil
	}
	var storeErr error
	if r.storeKey != nil {
		storeErr = r.storeKey.close()
	}
	return errors.Join(storeErr, r.category.close(), r.authority.close())
}
