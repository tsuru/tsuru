#!/bin/bash

DOCKER_TAG="latest"

if [ -n "${DOCKER_TAG}" ] && [ "${TRAVIS_GO_VERSION}" = "${GO_FOR_RELEASE}" ] && [ "${TRAVIS_BRANCH}" = "master" ]; then
	cat > ~/.dockercfg <<EOF
{
  "https://index.docker.io/v1/": {
    "auth": "${HUB_AUTH}",
    "email": "${HUB_EMAIL}"
  }
}
EOF
	docker build -t tsuru/api:${DOCKER_TAG} .
	docker push tsuru/api:${DOCKER_TAG}
else
	echo "No image to build"
fi
