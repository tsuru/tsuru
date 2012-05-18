#!/bin/sh -e

go test ./...
go build -o websrv ./api/webserver/main.go
./websrv -dry=true -config=${PWD}/etc/tsuru.conf
go build -o collect ./collector/main.go
./collect -dry=true
go build -o /tmp/tsuru ./cmd/main.go
out=$(/tmp/tsuru)
echo ${out} | grep -q '^Usage: tsuru' || exit 1
rm -f collect websrv /tmp/tsuru
