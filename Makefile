# Copyright 2012 tsuru authors. All rights reserved.
# Use of this source code is governed by a BSD-style
# license that can be found in the LICENSE file.

SHELL = /bin/bash -o pipefail
BUILD_DIR = build
TSR_BIN = $(BUILD_DIR)/tsurud
TSR_SRC = ./cmd/tsurud

.PHONY: all test race docs install tsurud $(TSR_BIN)

all: test

_go_test:
	go clean ./...
	go test ./... -check.v

_tsurud_dry:
	go build -o tsurud ./cmd/tsurud
	./tsurud api --dry --config ./etc/tsuru.conf
	rm -f tsurud

test: _go_test _tsurud_dry

leakdetector:
	go test -test.v --tags leakdetector ./... | tee /tmp/leaktest.log
	(cat /tmp/leaktest.log | grep LEAK) && exit 1 || exit 0

lint: metalint
	misc/check-contributors.sh

metalint:
	curl -sfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b $$(go env GOPATH)/bin v1.31.0
	echo "$$(go list ./... | grep -v /vendor/)" | sed 's|github.com/tsuru/tsuru/|./|' | xargs -t -n 4 \
		time golangci-lint run -c ./.golangci.yml

race:
	go test -race ./...

_install_api_doc:
	@go get github.com/tsuru/tsuru-api-docs

api-doc: _install_api_doc
	@tsuru-api-docs | grep -v missing > docs/handlers.yml

check-api-doc: _install_api_doc
	@exit $$(tsuru-api-docs | grep missing | wc -l)

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
	@sed -i "s/release = '.*'/release = '$(version)'/g" docs/conf.py
	@sed -i "s/version = '.*'/version = '$(MINOR)'/g" docs/conf.py
	@sed -i 's/const Version = ".*"/const Version = "$(version)"/' api/server.go

	@git add docs/conf.py api/server.go
	@git commit -m "bump to $(version)"

	@echo "Creating $(version) tag."
	@git tag $(version)

	@git push --tags
	@git push origin main

	@echo "$(version) released!"

install:
	go install ./...

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

validate-api-spec:
	cd / && GO111MODULE=on go get github.com/go-swagger/go-swagger/cmd/swagger@v0.22.0
	swagger validate ./docs/reference/api.yaml

test-int:
	go get -d github.com/tsuru/platforms/...
	TSURU_INTEGRATION_examplesdir="${GOPATH}/src/github.com/tsuru/platforms/examples" \
	TSURU_INTEGRATION_enabled=1 TSURU_INTEGRATION_verbose=2 TSURU_INTEGRATION_maxconcurrency=4 \
	TSURU_INTEGRATION_platforms="python" \
	TSURU_INTEGRATION_provisioners="docker" \
	go test -v -timeout 120m github.com/tsuru/tsuru/integration
