package toki

import (
	"errors"
	"fmt"
	"iter"

	"github.com/go-playground/locales"
	"golang.org/x/text/language"
)

type Gender uint8

const (
	_ Gender = iota

	GenderNeutral // They
	GenderMale    // He
	GenderFemale  // She
)

// Reader reads localized data.
type Reader interface {
	// Locale provides the locale this reader localizes for.
	Locale() language.Tag

	// Text provides a localized translation for the given TIK.
	Text(tik string, args ...any) (localized string)

	// Translator returns the localized translator of github.com/go-playground/locales
	// for the locale this reader localizes for.
	Translator() locales.Translator
}

type Localizer struct {
	locales          []language.Tag
	readers          []Reader
	defaultLocaleStr string
	matcher          language.Matcher
	readerByLocale   map[string]Reader
}

var (
	ErrEmptyBundle    = errors.New("bundle has no catalogs")
	ErrReaderConflict = errors.New("conflicting readers")
)

type Bundler interface {
	Catalogs() iter.Seq[Reader]
}

// New creates a new localizer.
func New(defaultLocale language.Tag, bundler Bundler) (*Localizer, error) {
	def := defaultLocale.String()
	var readers []Reader
	readerByLocale := make(map[string]Reader)
	var locales []language.Tag
	for r := range bundler.Catalogs() {
		locale := r.Locale()
		locales = append(locales, locale)
		localeStr := locale.String()
		if _, ok := readerByLocale[localeStr]; ok {
			return nil, fmt.Errorf("%w for %q", ErrReaderConflict, locale)
		}
		readerByLocale[localeStr] = r
		readers = append(readers, r)
	}
	if len(readers) < 1 {
		return nil, ErrEmptyBundle
	}
	matcher := language.NewMatcher(locales)
	return &Localizer{
		matcher:          matcher,
		locales:          locales,
		readers:          readers,
		defaultLocaleStr: def,
		readerByLocale:   readerByLocale,
	}, nil
}

// Match returns the best matching reader for locale.
func (l *Localizer) Match(locales ...language.Tag) (Reader, language.Confidence) {
	matchedTag, _, c := l.matcher.Match(locales...)
	return l.readerByLocale[matchedTag.String()], c
}

// ForBase returns either the localization for language, or the default localization
// if no localization for language is found.
func (l *Localizer) ForBase(language language.Base) Reader {
	r := l.readerByLocale[language.String()]
	if r == nil {
		r = l.readerByLocale[l.defaultLocaleStr]
	}
	return r
}

// Default returns the reader for the default locale.
func (l *Localizer) Default() Reader { return l.readerByLocale[l.defaultLocaleStr] }

// Locales returns all locales of the bundle.
func (l *Localizer) Locales() []language.Tag { return l.locales }

// Readers returns all available readers.
func (l *Localizer) Readers() []Reader { return l.readers }
