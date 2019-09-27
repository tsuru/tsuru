#!/bin/bash

set -o errexit
set -o nounset
set -o pipefail

SCRIPT_ROOT=$(dirname ${BASH_SOURCE})/..

bash vendor/k8s.io/code-generator/generate-groups.sh all \
  github.com/tsuru/tsuru/provision/kubernetes/pkg/client github.com/tsuru/tsuru/provision/kubernetes/pkg/apis \
  tsuru:v1 \
  --go-header-file ${SCRIPT_ROOT}/hack/boilerplate.go.txt

goimports -w ${SCRIPT_ROOT}/provision/kubernetes/pkg