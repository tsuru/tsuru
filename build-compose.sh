#!/bin/bash

# This script builds tsurud inside container and call compose to build and run a new api image.

docker run --rm -v "$PWD":/go/src/github.com/tsuru/tsuru -w /go/src/github.com/tsuru/tsuru golang:1.8 sh -c 'go build -o build/tsurud ./cmd/tsurud' && \
  docker-compose build api && \
  docker-compose up -d
