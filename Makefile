# Copyright 2012 tsuru authors. All rights reserved.
# Use of this source code is governed by a BSD-style
# license that can be found in the LICENSE file.

BUILD_DIR = build
TSR_BIN = $(BUILD_DIR)/tsurud
TSR_SRC = ./cmd/tsurud
TSR_PKGS = $$(go list ./... | grep -v /vendor/)

LINTER_ARGS_SLOW = \
	-j 4 --enable-gc -s vendor -e '.*/vendor/.*' --vendor --enable=misspell --enable=gofmt --enable=goimports --enable=unused \
	--disable=dupl --disable=gocyclo --disable=errcheck --disable=golint --disable=interfacer --disable=gas \
	--disable=structcheck --disable=gotype --disable=gotypex --deadline=60m --tests

LINTER_ARGS = \
	$(LINTER_ARGS_SLOW) --disable=staticcheck --disable=unused --disable=gosimple --disable=gosec


.PHONY: all check-path test race docs install

all: check-path test

# It does not support GOPATH with multiple paths.
check-path:
ifndef GOPATH
	@echo "FATAL: you must declare GOPATH environment variable, for more"
	@echo "       details, please check"
	@echo "       http://golang.org/doc/code.html#GOPATH"
	@exit 1
endif
ifneq ($(subst ~,$(HOME),$(GOPATH))/src/github.com/tsuru/tsuru, $(PWD))
	@echo "FATAL: you must clone tsuru inside your GOPATH To do so,"
	@echo "       you can run go get github.com/tsuru/tsuru/..."
	@echo "       or clone it manually to the dir $(GOPATH)/src/github.com/tsuru/tsuru"
	@exit 1
endif
	@exit 0

_go_test:
	go clean $(GO_EXTRAFLAGS) $(TSR_PKGS)
	go test $(GO_EXTRAFLAGS) $(TSR_PKGS) -check.v

_tsurud_dry:
	go build $(GO_EXTRAFLAGS) -o tsurud ./cmd/tsurud
	./tsurud api --dry --config ./etc/tsuru.conf
	rm -f tsurud

test: _go_test _tsurud_dry

leakdetector:
	go list ./... | xargs -I {} bash -c 'pushd $(GOPATH)/src/{}; go test --tags leakdetector; popd' | tee /tmp/leaktest.log
	(cat /tmp/leaktest.log | grep LEAK) && exit 1 || exit 0

lint: metalint
	misc/check-contributors.sh

metalint:
	@if [ -z $$(go version | grep -o 'go1.5') ]; then \
		go get -u github.com/alecthomas/gometalinter; \
		gometalinter --install; \
		go install $(TSR_PKGS); \
		go test -i $(TSR_PKGS); \
		gometalinter $(LINTER_ARGS) ./...; \
	fi

race:
	go test $(GO_EXTRAFLAGS) -race -i $(TSR_PKGS)
	go test $(GO_EXTRAFLAGS) -race $(TSR_PKGS)

_install_api_doc:
	@go get $(GO_EXTRAFLAGS) github.com/tsuru/tsuru-api-docs

api-doc: _install_api_doc
	@tsuru-api-docs | grep -v missing > docs/handlers.yml

check-api-doc: _install_api_doc
	@exit $(tsuru-api-docs | grep missing | wc -l)

doc-deps:
	@pip install -r requirements.txt

doc: doc-deps
	@cd docs && make html SPHINXOPTS="-N -W"

docs: doc

release:
	@if [ ! $(version) ]; then \
		echo "version parameter is required... use: make release version=<value>"; \
		exit 1; \
	fi

	$(eval PATCH := $(shell echo $(version) | sed "s/^\([0-9]\{1,\}\.[0-9]\{1,\}\.[0-9]\{1,\}\).*/\1/"))
	$(eval MINOR := $(shell echo $(PATCH) | sed "s/^\([0-9]\{1,\}\.[0-9]\{1,\}\).*/\1/"))
	@if [ $(MINOR) == $(PATCH) ]; then \
		echo "invalid version"; \
		exit 1; \
	fi

	@if [ ! -f docs/releases/tsurud/$(PATCH).rst ]; then \
		echo "to release the $(version) version you should create a release notes for version $(PATCH) first."; \
		exit 1; \
	fi

	@echo "Releasing tsuru $(version) version."
	@echo "Replacing version string."
	@sed -i "" "s/release = '.*'/release = '$(version)'/g" docs/conf.py
	@sed -i "" "s/version = '.*'/version = '$(MINOR)'/g" docs/conf.py
	@sed -i "" 's/const Version = ".*"/const Version = "$(version)"/' api/server.go

	@git add docs/conf.py api/server.go
	@git commit -m "bump to $(version)"

	@echo "Creating $(version) tag."
	@git tag $(version)

	@git push --tags
	@git push origin master

	@echo "$(version) released!"

install:
	go install $(GO_EXTRAFLAGS) $(TSR_PKGS) $$(go list ../tsuru-client/... | grep -v /vendor/)

serve: run-tsurud-api

run: run-tsurud-api

binaries: tsurud

tsurud: $(TSR_BIN)

$(TSR_BIN):
	go build -o $(TSR_BIN) $(TSR_SRC)

run-tsurud-api: $(TSR_BIN)
	$(TSR_BIN) api

run-tsurud-token: $(TSR_BIN)
	$(TSR_BIN) token

test-int:
	go get -d github.com/tsuru/platforms/...
	TSURU_INTEGRATION_examplesdir="${GOPATH}/src/github.com/tsuru/platforms/examples" \
	TSURU_INTEGRATION_enabled=1 TSURU_INTEGRATION_verbose=2 TSURU_INTEGRATION_maxconcurrency=4 \
	TSURU_INTEGRATION_platforms="python" \
	TSURU_INTEGRATION_provisioners="docker" \
	go test -v -timeout 120m github.com/tsuru/tsuru/integration
