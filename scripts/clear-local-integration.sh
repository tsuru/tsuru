#!/usr/bin/env bash

# Copyright 2025 tsuru authors. All rights reserved.
# Use of this source code is governed by a BSD-style
# license that can be found in the LICENSE file.
set -euo pipefail

readonly KUBECTL=${KUBECTL:-kubectl}

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

remove_app_services() {
  serivces=$(kubectl get service -l tsuru.io/is-tsuru=true -o jsonpath='{.items[*].metadata.name}')
  for service in $serivces; do
    echo "Removing service $service..."
    kubectl delete service "$service"
  done
}

remove_app_deployments() {
  deployments=$(kubectl get deployment -l tsuru.io/is-tsuru=true -o jsonpath='{.items[*].metadata.name}')
  for deployment in $deployments; do
    echo "Removing deployment $deployment..."
    kubectl delete deployment "$deployment"
  done
}

remove_app_replicasets() {
  replicasets=$(kubectl get rs -l tsuru.io/is-tsuru=true -o jsonpath='{.items[*].metadata.name}')
  for replicaset in $replicasets; do
    echo "Removing replicaset $replicaset..."
    kubectl delete rs "$replicaset"
  done
}

remove_app_secrets() {
  secrets=$(kubectl get secret -l tsuru.io/is-tsuru=true -o jsonpath='{.items[*].metadata.name}')
  for secret in $secrets; do
    echo "Removing secret $secret"
    kubectl delete secret "$secret"
  done
}

remove_app_hpas() {
  hpas=$(kubectl get hpa -l tsuru.io/is-tsuru=true -o jsonpath='{.items[*].metadata.name}')
  for hpa in $hpas; do
    echo "Removing HPA $hpa..."
    kubectl delete hpa "$hpa"
  done
}

remove_cronjobs() {
  cronjobs=$(kubectl get cronjob -l tsuru.io/is-tsuru=true -o jsonpath='{.items[*].metadata.name}')
  for cronjob in $cronjobs; do
    echo "Removing cronjob $cronjob..."
    kubectl delete cronjob "$cronjob"
  done
}

remove_jobs() {
  jobs=$(kubectl get job -l tsuru.io/is-tsuru=true -o jsonpath='{.items[*].metadata.name}')
  for job in $jobs; do
    echo "Removing job $job..."
    kubectl delete job "$job"
  done
}

main() {
  check_kubectl_config

  # remove app versions on mongodb, container "mongo", database "tsuru", collection "app_versions"
  kubectl -n tsuru-system exec tsuru-mongodb-0 -- bash -c 'mongosh tsuru --eval "db.app_versions.drop()"'
  kubectl -n tsuru-system exec tsuru-mongodb-0 -- bash -c 'mongosh tsuru --eval "db.apps.drop()"'
  kubectl -n tsuru-system exec tsuru-mongodb-0 -- bash -c 'mongosh tsuru --eval "db.teams.drop()"'
  kubectl -n tsuru-system exec tsuru-mongodb-0 -- bash -c 'mongosh tsuru --eval "db.pool.drop()"'
  kubectl -n tsuru-system exec tsuru-mongodb-0 -- bash -c 'mongosh tsuru --eval "db.platform_images.drop()"'
  kubectl -n tsuru-system exec tsuru-mongodb-0 -- bash -c 'mongosh tsuru --eval "db.platforms.drop()"'
  kubectl -n tsuru-system exec tsuru-mongodb-0 -- bash -c 'mongosh tsuru --eval "db.jobs.drop()"'
  kubectl -n tsuru-system exec tsuru-mongodb-0 -- bash -c 'mongosh tsuru --eval "db.services.drop()"'
  kubectl -n tsuru-system exec tsuru-mongodb-0 -- bash -c 'mongosh tsuru --eval "db.service_instances.drop()"'
  kubectl -n tsuru-system exec tsuru-mongodb-0 -- bash -c 'mongosh tsuru --eval "db.events.drop()"'

  # remove k8s resources
  remove_app_services
  remove_app_deployments
  remove_app_replicasets
  remove_app_secrets
  remove_app_hpas
  remove_cronjobs
  remove_jobs

  appImages=$(minikube image ls | grep 5000 | grep app | grep -v "<none>" || true)
  if [ -n "$appImages" ]; then
    echo "Removing app images from minikube..."
    for appImage in $appImages; do
      minikube ssh -- docker rmi -f "$appImage"
    done
  fi

  jobImages=$(minikube image ls | grep ijob | grep -v "<none>" || true)
  if [ -n "$jobImages" ]; then
    echo "Removing job images from minikube..."
    echo "$jobImages"
    for jobImage in $jobImages; do
      minikube ssh -- docker rmi -f "$jobImage"
    done
  fi
}

main
