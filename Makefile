# Copyright 2016 tsuru authors. All rights reserved.
# Use of this source code is governed by a BSD-style
# license that can be found in the LICENSE file.

BUILD_DIR = build
TSR_BIN = $(BUILD_DIR)/tsurud
TSR_SRC = cmd/tsurud/*.go

.PHONY: all check-path test race docs

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
	go clean $(GO_EXTRAFLAGS) ./...
	go test $(GO_EXTRAFLAGS) ./...

_tsurud_dry:
	go build $(GO_EXTRAFLAGS) -o tsurud ./cmd/tsurud
	./tsurud api --dry --config ./etc/tsuru.conf
	rm -f tsurud

test: _go_test _tsurud_dry

_install_deadcode: git
	go get $(GO_EXTRAFLAGS) github.com/reillywatson/go-misc/deadcode

deadcode: _install_deadcode
	@go list ./... | sed -e 's;github.com/tsuru/tsuru/;;' | grep -v vendor/ | xargs deadcode

deadc0de: deadcode

lint: deadcode
	./check-fmt.sh
	misc/check-license.sh
	misc/check-contributors.sh

race:
	go test $(GO_EXTRAFLAGS) -race -i ./...
	go test $(GO_EXTRAFLAGS) -race ./...

doc:
	@cd docs && make html SPHINXOPTS="-N -W"

docs: doc

release:
	@if [ ! $(version) ]; then \
		echo "version parameter is required... use: make release version=<value>"; \
		exit 1; \
	fi

	@if [ ! -f docs/releases/tsurud/$(version).rst ]; then \
		echo "to release the $(version) version you should create a release notes first."; \
		exit 1; \
	fi

	@echo "Releasing tsuru $(version) version."

	$(eval MAJOR := $(shell echo $(version) | sed "s/^\([0-9][0-9]*\.[0-9][0-9]*\).*/\1/"))

	@echo "Replacing version string."
	@sed -i "" "s/release = '.*'/release = '$(version)'/g" docs/conf.py
	@sed -i "" "s/version = '.*'/version = '$(MAJOR)'/g" docs/conf.py
	@sed -i "" 's/.tsurud., .[^,]*,/"tsurud", "$(version)",/' cmd/tsurud/main.go

	@git add docs/conf.py cmd/tsurud/main.go
	@git commit -m "bump to $(version)"

	@echo "Creating $(version) tag."
	@git tag $(version)

	@git push --tags
	@git push origin master

	@echo "$(version) released!"

install:
	go install $(GO_EXTRAFLAGS) ./... ../tsuru-client/...

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
