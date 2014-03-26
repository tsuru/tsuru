# Copyright 2014 tsuru authors. All rights reserved.
# Use of this source code is governed by a BSD-style
# license that can be found in the LICENSE file.

define check-service
    @if [ "$(shell nc -z localhost $2 1>&2 2> /dev/null; echo $$?)" != "0" ]; \
    then  \
        echo "\nFATAL: Expected to find $1 running on port $2\n"; \
        exit 1; \
    fi

endef

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

.PHONY: all check-path get hg git bzr get-test get-prod test race client

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

get: hg git bzr get-prod get-test godep

hg:
	$(if $(shell hg), , $(error $(HG_ERROR)))

git:
	$(if $(shell git), , $(error $(GIT_ERROR)))

bzr:
	$(if $(shell bzr), , $(error $(BZR_ERROR)))

get-test:
	@/bin/echo -n "Installing test dependencies... "
	@go list -f '{{range .TestImports}}{{.}} {{end}}' ./... | tr ' ' '\n' |\
	       grep '^.*\..*/.*$$' | grep -v 'github.com/globocom/tsuru' |\
	       sort | uniq | xargs go get -u -d >/tmp/.get-test 2>&1 || (cat /tmp/.get-test && exit 1)
	@go list -f '{{range .XTestImports}}{{.}} {{end}}' ./... | tr ' ' '\n' |\
	       grep '^.*\..*/.*$$' | grep -v 'github.com/globocom/tsuru' |\
	       sort | uniq | xargs go get -u -d >/tmp/.get-test 2>&1 || (cat /tmp/.get-test && exit 1)
	@/bin/echo "ok"
	@rm -f /tmp/.get-test

get-prod:
	@/bin/echo -n "Installing production dependencies... "
	@go get -u -d ./... 1>/tmp/.get-prod 2>&1 || (cat /tmp/.get-prod && exit 1)
	@/bin/echo "ok"
	@rm -f /tmp/.get-prod

godep:
	go get github.com/tools/godep
	godep restore ./...
	godep go clean ./...

check-test-services:
	$(call check-service,MongoDB,27017)
	$(call check-service,Redis,6379)
	$(call check-service,Beanstalk,11300)

_go_test:
	@go clean ./...
	@godep go test ./...

_tsr_dry:
	@godep go build -o tsr ./cmd/tsr
	@./tsr api --dry --config ./etc/tsuru.conf
	@./tsr collector --dry --config ./etc/tsuru.conf
	@rm -f tsr

_sh_tests:
	@cmd/term/test.sh
	@misc/test-hooks.bash

test: _go_test _tsr_dry _sh_tests

_travis_go_test:
	go clean ./...
	/bin/bash -ec 'for pkg in `go list ./...`; do go test -v $$pkg; done'

travis_test: _travis_go_test _tsr_dry _sh_tests

_install_deadcode: git
	@go get github.com/remyoudompheng/go-misc/deadcode

deadcode: _install_deadcode
	@go list ./... | sed -e 's;github.com/globocom/tsuru/;;' | xargs deadcode

race:
	@/bin/bash -ec 'for pkg in `go list ./...`; do go test -race -i $$pkg; go test -race $$pkg; done'

doc:
	@cd docs && make html SPHINXOPTS="-N -W"
