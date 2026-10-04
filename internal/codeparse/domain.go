package codeparse

import (
	"errors"
	"fmt"
	"iter"
	"os"
	"path/filepath"
	"strings"

	"github.com/goccy/go-yaml"
)

const DomainFileName = ".tokidomain.yml"

var (
	ErrDomainNameEmpty   = errors.New("domain name must not be empty")
	ErrDomainFileInvalid = errors.New("invalid .tokidomain.yml file")
)

// Domain represents a TIK domain defined by a .tokidomain file.
type Domain struct {
	// Name is required, non-empty.
	Name string

	// Description is optional.
	Description string

	// Dir holds the absolute path to the directory containing the .tokidomain file.
	Dir string

	// Parent is nil for the root domain.
	Parent *Domain

	// SubDomains holds the direct child domains.
	SubDomains []*Domain
}

// Path returns an iterator over the domain chain from this domain down to the root.
func (d *Domain) Path() iter.Seq[*Domain] {
	return func(yield func(*Domain) bool) {
		for d := d; d != nil; d = d.Parent {
			if !yield(d) {
				return
			}
		}
	}
}

// DomainTree holds all discovered domains and provides lookup by directory.
type DomainTree struct {
	byDir map[string]*Domain // Absolute directory path -> Domain defined there.
}

// Len returns the number of domains in the tree.
func (dt *DomainTree) Len() int { return len(dt.byDir) }

// All returns an iterator over all domains in the tree.
func (dt *DomainTree) All() iter.Seq[*Domain] {
	return func(yield func(*Domain) bool) {
		for _, d := range dt.byDir {
			if !yield(d) {
				return
			}
		}
	}
}

// ForDir returns the domain that applies to dir by walking up to the nearest
// ancestor with a .tokidomain file. Returns nil if no domain covers dir.
func (dt *DomainTree) ForDir(dir string) *Domain {
	for d := dir; ; {
		if dom, ok := dt.byDir[d]; ok {
			return dom
		}
		parent := filepath.Dir(d)
		if parent == d {
			return nil // Reached filesystem root.
		}
		d = parent
	}
}

type domainFile struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
}

func parseDomainFile(path string) (*domainFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var df domainFile
	if err := yaml.Unmarshal(data, &df); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrDomainFileInvalid, err)
	}
	df.Name = strings.TrimSpace(df.Name)
	if df.Name == "" {
		return nil, ErrDomainNameEmpty
	}
	return &df, nil
}

// DiscoverDomains walks root and builds a DomainTree from all .tokidomain files found.
func DiscoverDomains(root string) (*DomainTree, error) {
	root, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}

	// First pass: collect all domain files.
	type entry struct {
		dir string
		df  *domainFile
	}
	var entries []entry

	err = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil // Skip inaccessible directories.
		}
		if d.IsDir() {
			return nil
		}
		if d.Name() != DomainFileName {
			return nil
		}
		dir := filepath.Dir(path)
		df, parseErr := parseDomainFile(path)
		if parseErr != nil {
			return fmt.Errorf("parsing %s: %w", path, parseErr)
		}
		entries = append(entries, entry{dir: dir, df: df})
		return nil
	})
	if err != nil {
		return nil, err
	}

	// Build the tree: create Domain objects and resolve parents.
	byDir := make(map[string]*Domain, len(entries))
	for _, e := range entries {
		byDir[e.dir] = &Domain{
			Name:        e.df.Name,
			Description: e.df.Description,
			Dir:         e.dir,
		}
	}

	// Second pass: resolve parent/child pointers by walking up the directory tree.
	for dir, dom := range byDir {
		d := filepath.Dir(dir)
		for d != dir {
			if parent, ok := byDir[d]; ok {
				dom.Parent = parent
				parent.SubDomains = append(parent.SubDomains, dom)
				break
			}
			prev := d
			d = filepath.Dir(d)
			if d == prev {
				break
			}
		}
	}

	return &DomainTree{byDir: byDir}, nil
}
