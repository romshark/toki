package app

import (
	"net/http"
	"slices"
	"strings"

	"github.com/a-h/templ"

	"github.com/romshark/toki/editor/app/template"
	"github.com/romshark/toki/editor/datapagesgen/href"
	"github.com/romshark/toki/internal/codeparse"
)

// PageDomains is /domains
type PageDomains struct{ App *App }

func (p PageDomains) GET(
	r *http.Request,
) (body templ.Component, redirect string, err error) {
	p.App.mu.Lock()
	defer p.App.mu.Unlock()

	if p.App.building {
		return nil, href.PageBuildBundle(), nil
	}
	if p.App.dir == "" || p.App.initErr != "" {
		return nil, href.PageProjectDir(), nil
	}

	data := p.App.buildDomainData()
	return template.PageDomains(data), "", nil
}

func (a *App) buildDomainData() template.DataDomains {
	data := template.DataDomains{
		Dir:             a.dir,
		TotalChanges:    len(a.changed),
		CanApplyChanges: a.canApplyChangesLocked(),
	}

	if a.domains == nil {
		return data
	}

	// Build per-domain stats from template TIKs.
	// Map TIK ID -> domain dir (only available with full scan).
	tikDomainDir := make(map[string]string)
	if a.scan != nil {
		for i := range a.scan.Texts.Len() {
			t := a.scan.Texts.At(i)
			if t.Domain != nil {
				tikDomainDir[t.IDHash] = t.Domain.Dir
			}
		}
	}

	type domainStats struct {
		numTIKs, complete, incomplete, empty, invalid, changed int
	}
	statsByDir := make(map[string]*domainStats)

	for _, tk := range a.tiks {
		dir, ok := tikDomainDir[tk.ID]
		if !ok {
			continue
		}
		ds := statsByDir[dir]
		if ds == nil {
			ds = &domainStats{}
			statsByDir[dir] = ds
		}
		ds.numTIKs++
		if tk.IsChanged {
			ds.changed++
		}
		if tk.IsComplete {
			ds.complete++
		}
		if tk.IsIncomplete {
			ds.incomplete++
		}
		if tk.IsEmpty {
			ds.empty++
		}
		if tk.IsInvalid {
			ds.invalid++
		}
	}

	// Build domain info tree from the DomainTree roots.
	var buildInfo func(d *codeparse.Domain) template.DomainInfo
	buildInfo = func(d *codeparse.Domain) template.DomainInfo {
		// Build full name from path iterator.
		var names []string
		for p := range d.Path() {
			names = append(names, p.Name)
		}
		slices.Reverse(names)
		fullName := strings.Join(names, ".")

		info := template.DomainInfo{
			Name:        d.Name,
			Description: d.Description,
			Dir:         d.Dir,
			FullName:    fullName,
		}
		if d.Parent != nil {
			info.ParentName = d.Parent.Name
			var parentNames []string
			for p := range d.Parent.Path() {
				parentNames = append(parentNames, p.Name)
			}
			slices.Reverse(parentNames)
			info.ParentFullName = strings.Join(parentNames, ".")
		}
		if ds := statsByDir[d.Dir]; ds != nil {
			info.NumTIKs = ds.numTIKs
			info.NumComplete = ds.complete
			info.NumIncomplete = ds.incomplete
			info.NumEmpty = ds.empty
			info.NumInvalid = ds.invalid
			info.NumChanged = ds.changed
			if ds.numTIKs > 0 {
				info.Completeness = float64(ds.complete) / float64(ds.numTIKs)
			}
		}
		for _, sub := range d.SubDomains {
			info.SubDomains = append(info.SubDomains, buildInfo(sub))
		}
		return info
	}

	// Find root domains (no parent).
	for d := range a.domains.All() {
		if d.Parent == nil {
			data.Domains = append(data.Domains, buildInfo(d))
		}
	}

	data.TotalDomains = a.domains.Len()
	return data
}
