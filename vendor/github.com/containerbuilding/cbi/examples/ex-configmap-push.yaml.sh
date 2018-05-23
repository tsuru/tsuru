#!/bin/bash

set -o errexit
set -o nounset
set -o pipefail
set -o errtrace

if [ "$#" -ne 2 ]; then
    echo "Usage: $0 PUSH_TARGET DOCKER_REGISTRY_SECRET"
    exit 1
fi
PUSH_TARGET="$1"
DOCKER_REGISTRY_SECRET="$2"

out=$(dirname $0)/$(basename $0 | sed -e s/.yaml.sh/.generated.yaml/)
cat > ${out} << EOF
# Autogenarated by $0 at $(date)
apiVersion: v1
kind: ConfigMap
metadata:
  name: ex-configmap-push-configmap
data:
  Dockerfile: |-
    FROM busybox
    ADD hello /
    RUN cat /hello
  hello: "hello, pushing world"

---

apiVersion: cbi.containerbuilding.github.io/v1alpha1
kind: BuildJob
metadata:
  name: ex-configmap-push
spec:
  registry:
    target: ${PUSH_TARGET}
    push: true
    secretRef:
      name: ${DOCKER_REGISTRY_SECRET}
  language:
    kind: Dockerfile
  context:
    kind: ConfigMap
    configMapRef:
      name: ex-configmap-push-configmap
EOF
echo "generated ${out}"
