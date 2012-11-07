# Copyright 2012 tsuru authors. All rights reserved.
# Use of this source code is governed by a BSD-style
# license that can be found in the LICENSE file.

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
ifneq ($(subst ~,$(HOME),$(GOPATH))/src/github.com/globocom/tsuru, $(PWD))
	@echo "FATAL: you must clone tsuru inside your GOPATH To do so,"
	@echo "       you can run go get github.com/globocom/tsuru/..."
	@echo "       or clone it manually to the dir $(GOPATH)/src/github.com/globocom/tsuru"
	@exit 1
endif

get: hg git bzr libgit2 get-test get-prod

hg:
	$(if $(shell hg), , $(error $(HG_ERROR)))

git:
	$(if $(shell git), , $(error $(GIT_ERROR)))

bzr:
	$(if $(shell bzr), , $(error $(BZR_ERROR)))

libgit2:
	@pkg-config --libs libgit2 >/dev/null 2>&1 || echo "You need to install libgit2. Please visit http://libgit2.github.com/ for further instructions"

get-test:
	@/bin/echo -n "Installing test dependencies... "
	@go list -f '{{range .TestImports}}{{.}} {{end}}' ./... | tr ' ' '\n' |\
		grep '^.*\..*/.*$$' | grep -v 'github.com/globocom/tsuru' |\
		sort | uniq | xargs go get -u >/dev/null 2>&1
	@go list -f '{{range .XTestImports}}{{.}} {{end}}' ./... | tr ' ' '\n' |\
		grep '^.*\..*/.*$$' | grep -v 'github.com/globocom/tsuru' |\
		sort | uniq | xargs go get -u >/dev/null 2>&1
	@/bin/echo "ok"

get-prod:
	@/bin/echo -n "Installing production dependencies... "
	@go list -f '{{range .Imports}}{{.}} {{end}}' ./... | tr ' ' '\n' |\
		grep '^.*\..*/.*$$' | grep -v 'github.com/globocom/tsuru' |\
		sort | uniq | xargs go get -u >/dev/null 2>&1
	@/bin/echo "ok"

test:
	@go test -i ./...
	@go test ./...
	@go build -o websrv ./api/webserver
	@./websrv -dry=true -config=$(PWD)/etc/tsuru.conf
	@go build -o collect ./collector/
	@./collect -dry=true -config=$(PWD)/etc/tsuru.conf
	@rm -f collect websrv

doc:
	@cd docs && make html

client: libgit2
	@go build -o tsuru ./cmd/tsuru
	@echo "Copy tsuru to your binary path"
