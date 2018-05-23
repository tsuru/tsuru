#!/bin/bash

set -o errexit
set -o nounset
set -o pipefail
set -o errtrace

set -x

if [ "$#" -ne 2 ]; then
    echo "Usage: $0 REGISTRY TAG"
    exit 1
fi
REGISTRY="$1"
TAG="$2"

if echo ${REGISTRY} | grep '/$' > /dev/null; then
    echo "REGISTRY must not contain a trailing slash".
    exit 1
fi

cd $(dirname $0)/../..

DOCKER_BUILD_FLAGS=

# TODO: compare version as well
if [[ `docker info --format '{{json .ExperimentalBuild}}'` = true ]]; then
    export BUILD_STREAM_PROTOCOL=diffcopy
    DOCKER_BUILD_FLAGS="--stream"
fi

build() {
    t="$1"
    docker build -q -t ${REGISTRY}/${t}:${TAG} -f Dockerfile.${t} ${DOCKER_BUILD_FLAGS} .
}

push() {
    t="$1"
    docker push ${REGISTRY}/${t}:${TAG}
}

buildpush() {
    build $@
    push $@
}

# Build cbid image
build cbid
push cbid &

# Build rest images in parallel, after the first build gets completed, so as to exploit cache.
for t in $(ls Dockerfile.* | grep -v cbid | sed -e s/Dockerfile\.//g); do
    buildpush $t &
done
wait

# Generate and apply the manifest
yaml="/tmp/cbi.generated.yaml "
go run ./cmd/cbihack/*.go generate-manifests ${REGISTRY} ${TAG} > ${yaml}
kubectl apply -f ${yaml}
