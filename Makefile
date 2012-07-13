define HG_ERROR

FATAL: you need mercurial (hg) to download tsuru dependencies.
       Check INSTALL.md for details
endef

define GIT_ERROR

FATAL: you need git to download tsuru dependencies.
       Check INSTALL.md for details
endef

define BZR_ERROR

FATAL: you need bazaar (bzr) to download tsuru dependencies.
       Check INSTALL.md for details
endef

all: check-path get test

# It does not support GOPATH with multiple paths.
check-path:
ifndef GOPATH
	@echo "FATAL: you must declare GOPATH environment variable, for more"
	@echo "       details, please check INSTALL.md file and/or"
	@echo "       http://golang.org/cmd/go/#GOPATH_environment_variable"
	@exit 1
endif
ifneq ($(GOPATH)/src/github.com/timeredbull/tsuru, $(PWD))
	@echo "FATAL: you must clone tsuru inside your GOPATH To do so,"
	@echo "       you can run go get github.com/timeredbull/tsuru/..."
	@echo "       or clone it manually to the dir $(GOPATH)/src/github.com/timeredbull/tsuru"
	@exit 1
endif

get: hg git bzr get-test get-prod

hg:
	$(if $(shell hg), , $(error $(HG_ERROR)))

git:
	$(if $(shell git), , $(error $(GIT_ERROR)))

bzr:
	$(if $(shell bzr), , $(error $(BZR_ERROR)))

get-test:
	@/bin/echo -n "Installing test dependencies... "
	@go list -f '{{range .TestImports}}{{.}}\
		{{end}}' ./... | grep '^.*\..*/.*$$' | grep -v 'github.com/timeredbull/tsuru' | sort | uniq | sed -e 's/\\s//g' |\
		sed -e 's/\\//g' | xargs go get >/dev/null 2>&1
	@/bin/echo "ok"

get-prod:
	@/bin/echo -n "Installing production dependencies... "
	@go list -f '{{range .Imports}}{{.}}\
		{{end}}' ./... | grep '^.*\..*/.*$$' | grep -v 'github.com/timeredbull/tsuru' | sort | uniq | sed -e 's/\\s//g' |\
		sed -e 's/\\//g' | xargs go get >/dev/null 2>&1
	@/bin/echo "ok"

test:
	@go test -i ./...
	@go test ./...
	@go build -o websrv ./api/webserver
	@./websrv -dry=true -config=$(PWD)/etc/tsuru.conf
	@go build -o collect ./collector/
	@./collect -dry=true -config=$(PWD)/etc/tsuru.conf
	@rm -f collect websrv
