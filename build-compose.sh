#!/bin/bash

# This script builds tsurud inside container and call compose to build and run a new api image.

set -e

IP="10.200.10.1"
if ! `ping -c 1 -t 1 "$IP" > /dev/null`
then
  CURRENT_OS="$(uname)"
  if [[ "$CURRENT_OS" == "Linux" ]]; then
    sudo ip addr add dev docker0 $IP/24 || true
  elif [[ "$CURRENT_OS" == "Darwin" ]]; then
    sudo ifconfig lo0 alias $IP/24 || true
  else
    echo "Unsupported OS"
    exit 1
  fi
fi

docker run --rm -v "$PWD":/go/src/github.com/tsuru/tsuru -w /go/src/github.com/tsuru/tsuru golang:1.8-alpine sh -c 'CGO_ENABLED=0 go build -o build/tsurud ./cmd/tsurud'
docker-compose build api
docker-compose up -d
