#!/bin/bash

set -o errexit
set -o nounset
set -o pipefail
set -o errtrace

set -x

# for grepping kubectl result
export LANG=C LC_ALL=C

if [ "$#" -ne 1 ]; then
    echo "Usage: $0 REGISTRY"
    exit 1
fi
REGISTRY="$1"

if echo ${REGISTRY} | grep '/$' > /dev/null; then
    echo "REGISTRY must not contain a trailing slash".
    exit 1
fi

cd $(dirname $0)/../..

tag="test-$(date +%s)"
./hack/build/build-push-apply.sh ${REGISTRY} ${tag}

until kubectl get crd buildjobs.cbi.containerbuilding.github.io; do
    sleep 10
done
# cargo-cult extra sleep for avoiding `unable to recognize "STDIN": no matches for cbi.containerbuilding.github.io/, Kind=BuildJob`
sleep 10
until kubectl get buildjobs; do
    sleep 10
done

# TODO: move to golang
function e2e(){
    local ex=$1
    local plugin=$2
    echo "travis_fold:start:${ex}-${plugin}"
    echo "========== Testing ${ex} using ${plugin} plugin =========="
    # create a BuildJob
    (cat examples/${ex}.yaml; echo "  pluginSelector: plugin.name=${plugin}") | kubectl create -f -
    jobname=${ex}-job
    # wait for the underlying job
    pod=
    while [[ -z $pod ]]; do
        pod=$(kubectl get pods --selector=job-name=${jobname} --show-all --output=jsonpath={.items..metadata.name})
        sleep 10
    done
    until kubectl logs ${pod} > /dev/null 2>&1; do sleep 10; done
    # show the log and wait for completion
    kubectl logs -f ${pod}
    succeeded=
    while [[ -z ${succeeded} ]]; do
        succeeded=$(kubectl get job ${jobname}  --output=jsonpath={.status.succeeded})
        sleep 10
    done
    echo "Succeeded: ${succeeded}"
    [[ ${succeeded} = 1 ]]
    # delete the BuildJob
    kubectl delete buildjob ${ex}
    echo "travis_fold:end:${ex}-${plugin}"
}

# NOTE: no test for kaniko, because it always requires pushing at the moment.
for f in docker buildkit buildah img; do
    e2e ex-git-nopush $f
    e2e ex-http-nopush $f
    e2e ex-configmap-nopush $f
    kubectl delete configmap ex-configmap-nopush-configmap
done

# TODO: add tests for pushing
