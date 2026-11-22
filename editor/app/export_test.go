package app

import "github.com/romshark/toki/editor/app/template"

// Accessors for the tests in package app_test.

func (a *App) TIKs() []*template.TIK { return a.tiks }

func (a *App) NumMissing() int { return a.numMissing }

func (a *App) Changed() []*template.ICUMessage { return a.changed }

// LoadedFromIndexDB reports whether the last init took the fast path,
// which restores state without scanning the source.
func (a *App) LoadedFromIndexDB() bool { return a.scan == nil }

func (a *App) SetMessage(id, locale, message string) error {
	return a.setMessage(id, locale, message)
}
