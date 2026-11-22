// Package agentskills writes the skills that teach AI coding agents to use
// Toki in a Go module: toki for writing TIKs and toki-translate for
// translating the catalogs.
//
// Toki owns the skill files. Every write replaces them, the way
// `toki generate` replaces the generated Go files of the bundle.
package agentskills

import (
	"bytes"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"text/template"
)

// dirs are the skill directories relative to the module root: the one
// Claude Code reads and the tool-neutral one Codex and other agents read.
var dirs = []string{".claude/skills", ".agents/skills"}

// names lists the skills. skills/<name>.md is the template of each.
var names = []string{"toki", "toki-translate"}

//go:embed skills
var templates embed.FS

var tmpl = template.Must(template.ParseFS(templates, "skills/*"))

// Project is what the skills tell agents about the module.
type Project struct {
	// BundleDir is the bundle package relative to the module root,
	// separated by slashes, as "tokibundle".
	BundleDir string

	// BundleImportPath is the import path of the bundle package.
	BundleImportPath string

	// DefaultLocale is the locale of the TIKs, as "en".
	DefaultLocale string

	// NativeCatalog is the file name of the catalog of DefaultLocale,
	// as "catalog_en.arb".
	NativeCatalog string

	// Flags holds the flags toki commands need in the module, each with
	// a leading space, as " -b internal/i18n". It's empty for the defaults.
	Flags string

	// Version is the release of Toki writing the skills, as "v1.2.3".
	// It's empty for a build that isn't a release.
	Version string
}

// BundlePkg returns the name of the bundle package.
func (p Project) BundlePkg() string { return path.Base(p.BundleDir) }

// Write writes the skills for p into every directory of dirs in moduleDir.
// It returns the files whose content it changed, relative to moduleDir.
func Write(moduleDir string, p Project) (written []string, err error) {
	var buf bytes.Buffer
	for _, name := range names {
		buf.Reset()
		if err := tmpl.ExecuteTemplate(&buf, name+".md", p); err != nil {
			return written, fmt.Errorf("rendering skill %s: %w", name, err)
		}
		for _, dir := range dirs {
			rel := filepath.Join(filepath.FromSlash(dir), name, "SKILL.md")
			changed, err := writeFile(filepath.Join(moduleDir, rel), buf.Bytes())
			if err != nil {
				return written, err
			}
			if changed {
				written = append(written, rel)
			}
		}
	}
	return written, nil
}

// writeFile writes content to file unless the file holds it already.
// It reports whether it wrote.
func writeFile(file string, content []byte) (bool, error) {
	switch old, err := os.ReadFile(file); {
	case err == nil && bytes.Equal(old, content):
		return false, nil
	case err != nil && !errors.Is(err, fs.ErrNotExist):
		return false, err
	}
	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		return false, err
	}
	if err := os.WriteFile(file, content, 0o644); err != nil {
		return false, err
	}
	return true, nil
}
