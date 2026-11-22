# all regenerates the templates, then runs every check. As the first target,
# it is what a plain `make` runs.
all: templ test vulncheck

vulncheck:
	CGO_ENABLED=0 go run golang.org/x/vuln/cmd/govulncheck@latest ./...

fmt:
	go run mvdan.cc/gofumpt@latest -w .

fmtcheck:
	@unformatted=$$(go run mvdan.cc/gofumpt@latest -l .); \
	if [ -n "$$unformatted" ]; then \
		echo "Files not gofumpt formatted:"; \
		echo "$$unformatted"; \
		exit 1; \
	fi

lint:
	go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest run ./...

test: fmtcheck lint
	go test -coverpkg=./... -v .

templ:
	go run github.com/a-h/templ/cmd/templ@v0.3.1118 generate

# MORPHEUS_COMPONENTS lists the Morpheus components the editor uses.
# Add a component when a template starts using it, then run `make morpheus`.
MORPHEUS_COMPONENTS = alert badge breadcrumb button buttongroup card clipcopy collapsible \
	combobox keys layout link navgroup pagination popover radio-group resizable \
	select sidebar sortable switch textinput tooltip

# morpheus bundles MORPHEUS_COMPONENTS into the editor's static assets
# at the Morpheus version go.mod requires.
morpheus:
	go run github.com/romshark/morpheus@$$(go list -m -f '{{.Version}}' github.com/romshark/morpheus) \
		bundle $(addprefix -components ,$(MORPHEUS_COMPONENTS)) -o editor/app/static
	mv editor/app/static/morpheus.min.js editor/app/static/morpheus.js
	mv editor/app/static/morpheus.min.css editor/app/static/morpheus.css

# TEMPL_DEV_MODE_WATCH_ROOT scopes templ's hot-reload string lookup to this repository.
# Templier runs the app with TEMPL_DEV_MODE=true, which makes the templ runtime resolve
# every literal through a _templ.txt sidecar — including for prebuilt _templ.go files in
# the read-only module cache (Morpheus), which have none.
# Files outside the root fall back to their compiled-in literals.
dev-editor: templ
	cd editor/js && npm install
	TEMPL_DEV_MODE_WATCH_ROOT=$(CURDIR) \
		go run github.com/romshark/datapages/cmd/datapages@v0.11.4 watch

gen-example-large:
	go run ./cmd/genexamplelarge

DOCPAGES = go run github.com/romshark/docpages/cmd/docpages@v0.4.0

# docs writes the documentation site into site/ and fails on mistakes
# such as a broken link.
.PHONY: docs
docs:
	$(DOCPAGES) -o site docs

# docs-serve previews the documentation site and reloads it on changes.
docs-serve:
	$(DOCPAGES) -serve 127.0.0.1:7340 docs
