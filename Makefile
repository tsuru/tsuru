# Copyright 2012 tsuru authors. All rights reserved.
# Use of this source code is governed by a BSD-style
# license that can be found in the LICENSE file.

SHELL = /bin/bash -o pipefail
BUILD_DIR = build
TSR_BIN = $(BUILD_DIR)/tsurud
TSR_SRC = ./cmd/tsurud
K8S_VERSION=v1.20.0

ifeq (, $(shell go env GOBIN))
GOBIN := $(shell go env GOPATH)/bin
else
GOBIN := $(shell go env GOBIN)
endif

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

lint: metalint yamllint
	misc/check-contributors.sh

metalint:
	curl -sfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b $$(go env GOPATH)/bin v1.45.2
	echo "$$(go list ./... | grep -v /vendor/)" | sed 's|github.com/tsuru/tsuru/|./|' | xargs -t -n 4 \
		time golangci-lint run -c ./.golangci.yml

yamlfmt: ## Format your code with yamlfmt
ifeq (, $(shell which yamlfmt))
	go install github.com/google/yamlfmt/cmd/yamlfmt@v0.9.0
endif
	yamlfmt .

yamllint: ## Check the yaml is valid and correctly formatted
ifeq (, $(shell which yamlfmt))
	go install github.com/google/yamlfmt/cmd/yamlfmt@v0.9.0
endif
	@echo "yamlfmt --quiet --lint ."
	@yamlfmt --quiet --lint . \
		|| ( echo "Please run 'make yamlfmt' to fix it (if a format error)" && exit 1 )

race:
	go test -race ./...

_install_api_doc:
	@go install github.com/tsuru/tsuru-api-docs@latest

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

	$(eval SED := $(shell which gsed || which sed))
	$(eval PATCH := $(shell echo $(version) | $(SED) "s/^\([0-9]\{1,\}\.[0-9]\{1,\}\.[0-9]\{1,\}\).*/\1/"))
	$(eval MINOR := $(shell echo $(PATCH) | $(SED) "s/^\([0-9]\{1,\}\.[0-9]\{1,\}\).*/\1/"))
	@if [ $(MINOR) == $(PATCH) ]; then \
		echo "invalid version"; \
		exit 1; \
	fi

	@echo "Releasing tsuru $(version) version."
	@echo "Replacing version string."
	@$(SED) -i "s/release = '.*'/release = '$(version)'/g" docs/conf.py
	@$(SED) -i "s/version = '.*'/version = '$(MINOR)'/g" docs/conf.py
	@$(SED) -i 's/const Version = ".*"/const Version = "$(version)"/' api/server.go
	@$(SED) -i 's/version: ".*"/version: "$(MINOR)"/' docs/reference/api.yaml

install:
	go install ./...

serve: run-tsurud-api

run: run-tsurud-api

binaries: tsurud

tsurud: $(TSR_BIN)

$(TSR_BIN):
	CGO_ENABLED=0 go build -trimpath -ldflags '-s -w -X github.com/tsuru/tsuru/cmd.GitHash=$(shell git rev-parse HEAD) -X github.com/tsuru/tsuru/api.Version=$(shell git describe --tags --abbrev=0)' -o $(TSR_BIN) $(TSR_SRC)

run-tsurud-api: $(TSR_BIN)
	$(TSR_BIN) api

run-tsurud-token: $(TSR_BIN)
	$(TSR_BIN) token

.PHONY: validate-api-spec
validate-api-spec: install-swagger
	$(SWAGGER) validate ./docs/reference/api.yaml

test-int:
	go get -d github.com/tsuru/platforms/...
	TSURU_INTEGRATION_examplesdir="${GOPATH}/src/github.com/tsuru/platforms/examples" \
	TSURU_INTEGRATION_enabled=1 TSURU_INTEGRATION_verbose=2 TSURU_INTEGRATION_maxconcurrency=4 \
	TSURU_INTEGRATION_platforms="python" \
	TSURU_INTEGRATION_provisioners="minikube" \
	go test -v -timeout 120m github.com/tsuru/tsuru/integration

generate-test-certs:
	openssl genrsa -out ./app/testdata/private.key 1024
	openssl req -new -x509 -sha256 -key ./app/testdata/private.key -subj '/CN=app.io' -addext 'subjectAltName = DNS:app.io' -out ./app/testdata/certificate.crt -days 3650
	cp ./app/testdata/private.key ./api/testdata/key.pem
	cp ./app/testdata/certificate.crt ./api/testdata/cert.pem

# reference for minikube macOS registry: https://minikube.sigs.k8s.io/docs/handbook/registry/#docker-on-macos
local-mac:
	minikube start --driver=virtualbox --kubernetes-version=$(K8S_VERSION)
	minikube addons enable registry
	docker run -d --rm --network=host alpine ash -c "apk add socat && socat TCP-LISTEN:5000,reuseaddr,fork TCP:$(minikube ip):5000"
	@make local-api

local-mac-m1:
	minikube start --driver=docker --alsologtostderr --kubernetes-version=$(K8S_VERSION)
	minikube addons enable registry
	docker run -d --rm --network=host alpine ash -c "apk add socat && socat TCP-LISTEN:5000,reuseaddr,fork TCP:$(minikube ip):5000"
	@make local-api

local:
	minikube start --driver=none --kubernetes-version=$(K8S_VERSION)
	@make local-api

local-api:
	docker-compose up -d
	go build -o $(TSR_BIN) $(TSR_SRC)
	$(TSR_BIN) api -c ./etc/tsuru-local.conf

.PHONY: install-swagger
install-swagger:
ifeq (, $(shell command -v swagger))
	@{ go install github.com/go-swagger/go-swagger/cmd/swagger@v0.30.3; }
SWAGGER=$(GOBIN)/swagger
else
SWAGGER=$(shell command -v swagger)
endif


PROTOC ?= protoc
.PHONY: generate
generate-grpc:
	$(PROTOC) --go_out=. --go_opt=paths=source_relative \
		--go-grpc_out=. --go-grpc_opt=paths=source_relative \
		types/tag/service.proto
