#!/bin/bash

set -o errexit
set -o nounset
set -o pipefail

CODE_GENERATOR_VERSION=$(cat go.sum | grep k8s.io/code-generator | awk '{print $2}' | grep -v 'go.mod')
echo "k8s.io/code-generator version is" $CODE_GENERATOR_VERSION
SCRIPT_ROOT=$(dirname ${BASH_SOURCE})/..

go mod download

export GOPATH="$HOME/go"

source ~/go/pkg/mod/k8s.io/code-generator@${CODE_GENERATOR_VERSION}/kube_codegen.sh

kube::codegen::gen_helpers \
--boilerplate ${SCRIPT_ROOT}/hack/boilerplate.go.txt \
${SCRIPT_ROOT}/provision/kubernetes/pkg/apis

goimports -w ${SCRIPT_ROOT}/provision/kubernetes/pkg
