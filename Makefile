vulncheck:
	go run golang.org/x/vuln/cmd/govulncheck@latest ./...

fmtcheck:
	@unformatted=$$(go run mvdan.cc/gofumpt@latest -l .); \
	if [ -n "$$unformatted" ]; then \
		echo "Files not gofumpt formatted:"; \
		echo "$$unformatted"; \
		exit 1; \
	fi

test: fmtcheck
	go test -coverpkg=./... -v .

templ:
	go run github.com/a-h/templ/cmd/templ@v0.3.906 generate
