#!/bin/bash

set -o errexit
set -o nounset
set -o pipefail
set -o errtrace

set -x

cd $(dirname $0)/../..
source ./hack/dind/config

# Download Mirantis/kubeadm-dind-cluster 
if [ ! -x ${DIND_CLUSTER_SH} ]; then
    wget -O ${DIND_CLUSTER_SH} ${DIND_CLUSTER_SH_URL}
    chmod +x ${DIND_CLUSTER_SH}
fi

# Start kube
DIND_INSECURE_REGISTRIES="[\"${CBI_REGISTRY}:5000\"]" DIND_DAEMON_JSON_FILE=/dev/null ${DIND_CLUSTER_SH} up

# Add registry to the network
docker run -d --network ${DIND_NET} --name ${CBI_REGISTRY} ${CBI_REGISTRY_IMAGE}

# Add bootstrap docker to the network, and expose the port
docker run -d --privileged --network ${DIND_NET} --name ${CBI_BOOTSTRAP_DOCKER} -p ${CBI_BOOTSTRAP_DOCKER_EXPOSE}:2375 ${CBI_BOOTSTRAP_DOCKER_IMAGE} --insecure-registry=${CBI_REGISTRY}:5000  --experimental
