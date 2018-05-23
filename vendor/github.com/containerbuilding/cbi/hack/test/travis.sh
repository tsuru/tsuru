#!/bin/bash

set -o errexit
set -o nounset
set -o pipefail
set -o errtrace

set -x

cd $(dirname $0)/../..

export PATH=~/.kubeadm-dind-cluster:$PATH

echo travis_fold:start:dind-up

# workaround on travis: kubernetes/kubernetes-anywhere#88
sudo mkdir /vlkp
sudo chmod 777 /vlkp
sudo mount -o bind /vlkp /vlkp
sudo mount --make-rshared /vlkp
export DIND_VARLIBKUBELETPODS_BASE=/vlkp

./hack/dind/up.sh
echo travis_fold:end:dind-up

DOCKER_HOST=localhost:62375 ./hack/test/e2e.sh cbi-registry:5000/cbi
# no need to call ./hack/dind/down.sh on travis
