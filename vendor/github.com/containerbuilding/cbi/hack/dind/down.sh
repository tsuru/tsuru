#!/bin/bash

set -o errexit
set -o nounset
set -o pipefail
set -o errtrace

set -x

cd $(dirname $0)/../..
source ./hack/dind/config

docker rm -f ${CBI_REGISTRY} ${CBI_BOOTSTRAP_DOCKER}
${DIND_CLUSTER_SH} down

