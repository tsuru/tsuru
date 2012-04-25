#!/bin/sh

set -e

go test ./...
for f in `find . -name main.go`
do
    go build $f
done
