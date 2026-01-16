#!/usr/bin/env bash

# Copyright 2024 tsuru authors. All rights reserved.
# Use of this source code is governed by a BSD-style
# license that can be found in the LICENSE file.
set -eu -o pipefail

[[ -n ${DEBUG:-} ]] && set -x

readonly DOCKER=${DOCKER:-docker}
readonly HELM=${HELM:-helm}
readonly KIND=${KIND:-kind}
readonly KUBECTL=${KUBECTL:-kubectl}
readonly BINDIR=${BINDIR:-./bin}
readonly TSURU=${TSURU:-${BINDIR}/tsuru}
readonly MINIKUBE=${MINIKUBE:-minikube}

readonly CLUSTER_PROVIDER=${CLUSTER_PROVIDER:-kind}
readonly NAMESPACE=${NAMESPACE:-tsuru-system}

readonly CHART_VERSION_TSURU_STACK=${CHART_VERSION_TSURU_STACK:-0.8.9}

function onerror() {
  set -e
  echo "TSURU API LOGS:"
  ${KUBECTL} logs -n ${NAMESPACE} deploy/tsuru-api || true
  echo
  ${KUBECTL} get pods -A -o wide
  echo
  ${KUBECTL} get services -A -o wide
  [[ -n ${tsuru_api_port_forward_pid} ]] && kill ${tsuru_api_port_forward_pid}
  [[ -n ${nginx_ingress_port_forward_pid} ]] && kill ${nginx_ingress_port_forward_pid}
  [[ -n ${minikube_tunnel_pid} ]] && kill ${minikube_tunnel_pid}
  set +e
  exit 1
}

function check_kubectl_config() {
  if [[ -z "${INTEGRATION_KUBECONFIG:-}" ]]; then
    echo "INTEGRATION_KUBECONFIG is not set. Aborting." >&2
    exit 1
  fi
  if [[ "${INTEGRATION_KUBECONFIG}" == "${HOME}/.kube/config" ]]; then
    echo "INTEGRATION_KUBECONFIG cannot be equal to your default kubeconfig (${HOME}/.kube/config). Aborting." >&2
    exit 1
  fi
  if [[ ! -f "${INTEGRATION_KUBECONFIG}" ]]; then
    echo "INTEGRATION_KUBECONFIG file (${INTEGRATION_KUBECONFIG}) does not exist. Aborting." >&2
    exit 1
  fi
  export KUBECONFIG="${INTEGRATION_KUBECONFIG}"
  if ! ${KUBECTL} config get-contexts -o name | grep -qE '^(minikube|kind-kind)$'; then
    echo "INTEGRATION_KUBECONFIG must have 'minikube' or 'kind-kind' context. Aborting." >&2
    exit 1
  fi
  if [[ "$(${KUBECTL} config get-contexts -o name | wc -l)" -ne 1 ]]; then
    echo "INTEGRATION_KUBECONFIG must have only one context. Aborting." >&2
    exit 1
  fi
}

install_tsuru_stack() {
  trap onerror ERR

  ${HELM} repo add --force-update tsuru https://tsuru.github.io/charts

  ${HELM} install --create-namespace \
    --namespace ${NAMESPACE} --version ${CHART_VERSION_TSURU_STACK} \
    --set tsuru-api.image.repository=localhost/tsuru/tsuru-api \
    --set tsuru-api.image.tag=integration \
    --set tsuru-api.image.pullPolicy=Never \
    --set tsuru-api.service.type=ClusterIP \
    --set tsuru-api.tsuruConfig.debug=true \
    --timeout 5m \
    tsuru tsuru/tsuru-stack
}

install_kedacore() {
  trap onerror ERR

  ${HELM} repo add --force-update kedacore https://kedacore.github.io/charts

  ${HELM} install keda kedacore/keda --namespace tsuru-system --version 2.11.1
}

build_tsuru_api_container_image() {
  ${DOCKER} build -t localhost/tsuru/tsuru-api:integration -f Dockerfile .

  case ${CLUSTER_PROVIDER} in
  minikube)
    ${DOCKER} save localhost/tsuru/tsuru-api:integration | ${MINIKUBE} image load -
    ;;

  kind)
    ${DOCKER} save "localhost/tsuru/tsuru-api:integration" -o "tsuru-api.tar"
    ${KIND} load image-archive "tsuru-api.tar"
    rm "tsuru-api.tar"
    ;;
  *)
    print "Invalid local cluster provider (got ${CLUSTER_PROVIDER}, supported: kind, minikube)" >&2
    exit 1
    ;;
  esac
}

set_initial_admin_password() {
  trap onerror ERR
  ${KUBECTL} exec -n ${NAMESPACE} deploy/tsuru-api -- \
    sh -c "echo $'123456\n123456' | /usr/local/bin/tsurud root user create admin@admin.com"
}

main() {
  trap onerror ERR

  check_kubectl_config

  ${KUBECTL} cluster-info
  ${KUBECTL} get all

  ${KUBECTL} get namespace ${NAMESPACE} >/dev/null 2>&1 ||
    ${KUBECTL} create namespace ${NAMESPACE}

  build_tsuru_api_container_image

  if [ "${CLUSTER_PROVIDER}" == "minikube" ]; then
    ${MINIKUBE} tunnel &
    minikube_tunnel_pid=${!}
  fi
  install_tsuru_stack
  install_kedacore

  sleep 30

  local_tsuru_api_port=8080
  DEBUG="" ${KUBECTL} -n ${NAMESPACE} port-forward svc/tsuru-api ${local_tsuru_api_port}:80 --address=127.0.0.1 &
  tsuru_api_port_forward_pid=${!}

  local_nginx_ingress_port=8890
  DEBUG="" ${KUBECTL} -n ${NAMESPACE} port-forward svc/tsuru-ingress-nginx-controller ${local_nginx_ingress_port}:80 --address=127.0.0.1 &
  nginx_ingress_port_forward_pid=${!}

  set_initial_admin_password

  if [ ! -d bin ]; then mkdir bin; fi
  curl -fsSL "https://tsuru.io/get" | bash -s -- -b ${BINDIR}

  export TSURU_TARGET="http://127.0.0.1:${local_tsuru_api_port}"
  echo "123456" | ${TSURU} login admin@admin.com

  PATH=$PATH:$PWD/bin make test-ci-integration

  [[ -n ${tsuru_api_port_forward_pid} ]] && kill ${tsuru_api_port_forward_pid}
  [[ -n ${nginx_ingress_port_forward_pid} ]] && kill ${nginx_ingress_port_forward_pid}
  [[ -n ${minikube_tunnel_pid} ]] && kill ${minikube_tunnel_pid}
}

main $@
