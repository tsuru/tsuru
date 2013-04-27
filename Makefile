# Copyright 2013 tsuru authors. All rights reserved.
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

get: hg git bzr get-test get-prod

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
		sort | uniq | xargs go get -u >/dev/null
	@go list -f '{{range .XTestImports}}{{.}} {{end}}' ./... | tr ' ' '\n' |\
		grep '^.*\..*/.*$$' | grep -v 'github.com/globocom/tsuru' |\
		sort | uniq | xargs go get -u >/dev/null
	@/bin/echo "ok"

get-prod:
	@/bin/echo -n "Installing production dependencies... "
	@go list -f '{{range .Imports}}{{.}} {{end}}' ./... | tr ' ' '\n' |\
		grep '^.*\..*/.*$$' | grep -v 'github.com/globocom/tsuru' |\
		sort | uniq | xargs go get -u >/dev/null
	@/bin/echo "ok"

test:
	@go test -i ./...
	@go test ./...
	@go build -o tsr ./cmd/tsr
	@./tsr api --dry --config ./etc/tsuru.conf
	@./tsr collector --dry --config ./etc/tsuru.conf
	@rm -f tsr
	@cmd/term/test.sh
	@misc/test-hooks.bash

race:
	@for pkg in `go list ./...`; do go test -race -i $$pkg; go test -race $$pkg; done

doc:
	@cd docs && make html

client:
	@go build -o tsuru ./cmd/tsuru
	@echo "Copy tsuru to your binary path"

# TODO: find a better place to do this
run-docker:
	@go build -o tsr ./cmd/tsr
	@sudo service beanstalkd start
	@./tsr api --dry --config ./etc/tsuru-docker.conf
	@./tsr collector --dry --config ./etc/tsuru-docker.conf
	@sudo su - git -c gandalf-webserver &
	@sudo su - git -c "git daemon --base-path=/var/repositories --syslog --export-all" &

stop:
	@if ps ax|grep "[s]udo su - git -c gandalf-webserver"; then ps ax|grep "[s]udo su - git -c gandalf-webserver"|awk {'print $$1'}|xargs sudo kill; fi
	@if ps ax|grep "[g]it-daemon"; then ps ax|grep "[g]it-daemon"|awk {'print $$1'}|xargs sudo kill; fi
	@if ps ax|grep "[w]ebsrv -config"; then ps ax|grep "[w]ebsrv -config"|awk {'print $$1'}|xargs kill; fi
	@if ps ax|grep "[c]ollect -config"; then ps ax|grep "[c]ollect -config"|awk {'print $$1'}|xargs kill; fi
	@sudo service beanstalkd stop
	@rm -f tsr
