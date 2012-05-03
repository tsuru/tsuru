#!/bin/sh

set -e

go test ./...
go build -o websrv ./api/webserver/main.go
./websrv -dry=true -config=${PWD}/etc/tsuru.conf
go build -o collect ./collector/main.go
./collect -dry=true
rm -f collect websrv
