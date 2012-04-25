#!/bin/sh

go test ./...
find . -name main.go -exec go build {} \;
