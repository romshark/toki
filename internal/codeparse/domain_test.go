package codeparse_test

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/romshark/toki/internal/codeparse"
	"github.com/stretchr/testify/require"
)

func writeDomainFile(t *testing.T, dir, content string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, codeparse.DomainFileName), []byte(content), 0o644,
	))
}

func domainNames(d *codeparse.Domain) []string {
	var names []string
	for d := range d.Path() {
		names = append(names, d.Name)
	}
	slices.Reverse(names)
	return names
}

func subdomainNames(d *codeparse.Domain) []string {
	var names []string
	for _, d := range d.SubDomains {
		names = append(names, d.Name)
	}
	return names
}

func TestDiscoverDomains(t *testing.T) {
	root := t.TempDir()

	writeDomainFile(t, root, "name: myapp\ndescription: root domain\n")
	writeDomainFile(t, filepath.Join(root, "api"), "name: api\ndescription: API layer\n")
	writeDomainFile(t, filepath.Join(root, "api", "auth"),
		"name: auth\ndescription: \"\"\n")

	// No domain file in api/billing; inherit from api.
	require.NoError(t, os.MkdirAll(filepath.Join(root, "api", "billing"), 0o755))

	dt, err := codeparse.DiscoverDomains(root)
	require.NoError(t, err)

	// Root domain.
	domainRoot := dt.ForDir(root)
	require.NotNil(t, domainRoot)
	require.Equal(t, "myapp", domainRoot.Name)
	require.Equal(t, "root domain", domainRoot.Description)
	require.Nil(t, domainRoot.Parent)
	require.Equal(t, []string{"myapp"}, domainNames(domainRoot))
	require.Equal(t, []string{"api"}, subdomainNames(domainRoot))

	// api domain.
	domainAPI := dt.ForDir(filepath.Join(root, "api"))
	require.NotNil(t, domainAPI)
	require.Equal(t, "api", domainAPI.Name)
	require.Equal(t, domainRoot, domainAPI.Parent)
	require.Equal(t, []string{"myapp", "api"}, domainNames(domainAPI))
	require.Equal(t, []string{"auth"}, subdomainNames(domainAPI))

	// api/auth domain.
	domainAuth := dt.ForDir(filepath.Join(root, "api", "auth"))
	require.NotNil(t, domainAuth)
	require.Equal(t, "auth", domainAuth.Name)
	require.Equal(t, domainAPI, domainAuth.Parent)
	require.Equal(t, []string{"myapp", "api", "auth"}, domainNames(domainAuth))
	require.Empty(t, subdomainNames(domainAuth))

	// api/billing inherits from api.
	domainBilling := dt.ForDir(filepath.Join(root, "api", "billing"))
	require.Equal(t, domainAPI, domainBilling)
}

func TestDiscoverDomainsNoDomainFiles(t *testing.T) {
	root := t.TempDir()
	dt, err := codeparse.DiscoverDomains(root)
	require.NoError(t, err)
	require.Nil(t, dt.ForDir(root))
}

func TestDiscoverDomainsErrEmptyName(t *testing.T) {
	root := t.TempDir()
	writeDomainFile(t, root, "name: \"\"\ndescription: bad\n")

	_, err := codeparse.DiscoverDomains(root)
	require.ErrorIs(t, err, codeparse.ErrDomainNameEmpty)
}

func TestDiscoverDomainsErrInvalidYAML(t *testing.T) {
	root := t.TempDir()
	writeDomainFile(t, root, "{{invalid yaml")

	_, err := codeparse.DiscoverDomains(root)
	require.ErrorIs(t, err, codeparse.ErrDomainFileInvalid)
}

func TestDomainPathNil(t *testing.T) {
	var d *codeparse.Domain
	var names []string
	for d := range d.Path() {
		names = append(names, d.Name)
	}
	require.Nil(t, names)
}
