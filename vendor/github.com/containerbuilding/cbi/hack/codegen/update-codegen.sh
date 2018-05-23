#!/bin/bash

# Copyright 2017 The Kubernetes Authors.
# https://github.com/kubernetes/sample-controller/tree/4d47428cc1926e6cc47f4a5cf4441077ca1b605f
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

set -o errexit
set -o nounset
set -o pipefail

SCRIPT_ROOT=$(dirname ${BASH_SOURCE})/../..

# FIXME: TBD: k8s.io/code-generator and some stuff needs to be installed
CODEGEN_PKG=${CODEGEN_PKG:-$(cd ${SCRIPT_ROOT}; ls -d -1 ./vendor/k8s.io/code-generator 2>/dev/null || echo ${SCRIPT_ROOT}/../../../k8s.io/code-generator)}

set -x

# generate the code with:
# --output-base    because this script should also be able to run inside the vendor dir of
#                  k8s.io/kubernetes. The output-base is needed for the generators to output into the vendor dir
#                  instead of the $GOPATH directly. For normal projects this can be dropped.
${CODEGEN_PKG}/generate-groups.sh "deepcopy,client,informer,lister" \
  github.com/containerbuilding/cbi/pkg/client github.com/containerbuilding/cbi/pkg/apis \
  cbi:v1alpha1 \
  --output-base "$(dirname ${BASH_SOURCE})/../../../../.." \
  --go-header-file ${SCRIPT_ROOT}/hack/codegen/boilerplate.go.txt


# generate plugin API
(
    cd ${SCRIPT_ROOT}/pkg/plugin/api
    go generate .
)

# generate cbi-latest.yaml
go run cmd/cbihack/*.go generate-manifests containerbuilding latest > cbi-latest.yaml
