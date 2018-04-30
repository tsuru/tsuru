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

if [[ ! $(docker images -q tsuru-build) ]]; then docker build -t tsuru-build -f Dockerfile.build .; fi;

BUILD_IMAGE='tsuru-build'

LOCAL_PKG=${GOPATH}'/pkg/linux_amd64'
CONTAINER_PKG='/go/pkg/linux_amd64'
CONTAINER_PROJECT_PATH='/go/src/github.com/tsuru/tsuru'
BUILD_CMD="go build -i -v --ldflags '-linkmode external -extldflags \"-static\"' -o build/tsurud ./cmd/tsurud"

set -x

docker run --rm -v ${LOCAL_PKG}:${CONTAINER_PKG} -v ${PWD}:${CONTAINER_PROJECT_PATH} -w ${CONTAINER_PROJECT_PATH} -e CC=/usr/bin/gcc -e GOPATH=/go ${BUILD_IMAGE} sh -c "${BUILD_CMD}"
docker-compose -f ${1:-"docker-compose.yml"} build api
docker-compose -f ${1:-"docker-compose.yml"} up -d
