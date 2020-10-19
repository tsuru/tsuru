#!/bin/bash

set -o errexit
set -o nounset
set -o pipefail

CODE_GENERATOR_VERSION=$(cat go.sum | grep k8s.io/code-generator | awk '{print $2}' | grep -v 'go.mod')
echo "k8s.io/code-generator version is" $CODE_GENERATOR_VERSION
SCRIPT_ROOT=$(dirname ${BASH_SOURCE})/..

go get k8s.io/code-generator@$CODE_GENERATOR_VERSION || test ok

bash ~/go/pkg/mod/k8s.io/code-generator@${CODE_GENERATOR_VERSION}/generate-groups.sh all \
  github.com/tsuru/tsuru/provision/kubernetes/pkg/client github.com/tsuru/tsuru/provision/kubernetes/pkg/apis \
  tsuru:v1 \
  --go-header-file ${SCRIPT_ROOT}/hack/boilerplate.go.txt

goimports -w ${SCRIPT_ROOT}/provision/kubernetes/pkg
