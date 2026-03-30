vulncheck:
	go run golang.org/x/vuln/cmd/govulncheck@latest ./...

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
	go run github.com/a-h/templ/cmd/templ@v0.3.1001 generate

dev-editor:
	datapages watch

gen-example-large:
	go run ./cmd/genexamplelarge
