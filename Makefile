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
	go run github.com/a-h/templ/cmd/templ@v0.3.1020 generate

# TEMPL_DEV_MODE_WATCH_ROOT scopes templ's hot-reload string lookup to this repository.
# Templier runs the app with TEMPL_DEV_MODE=true, which makes the templ runtime resolve
# every literal through a _templ.txt sidecar — including for prebuilt _templ.go files in
# the read-only module cache (Morpheus), which have none.
# Files outside the root fall back to their compiled-in literals.
dev-editor: templ
	cd editor/js && npm install
	TEMPL_DEV_MODE_WATCH_ROOT=$(CURDIR) \
		go run github.com/romshark/datapages/cmd/datapages@v0.10.1 watch

gen-example-large:
	go run ./cmd/genexamplelarge
