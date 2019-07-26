get: get-test

get-test:
	@/bin/echo "Installing test dependencies... "
	@go list -f '{{range .TestImports}}{{.}} {{end}}' ./ | tr ' ' '\n' |\
		grep '^.*\..*/.*$$' | grep -v 'github.com/globocom/go-gandalfclient' |\
		sort | uniq | xargs go get >/dev/null 2>&1
	@/bin/echo "ok"

test:
	@go test -i ./
	@go test ./
