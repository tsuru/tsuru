#!/bin/bash

DOCKER_TAG="latest"

if [ -n "$TRAVIS_TAG" ] && [[ "${TRAVIS_TAG}" =~ ([0-9]+). ]] && [[ $TRAVIS_TAG != *"rc"* ]]
then
    DOCKER_TAG=v${BASH_REMATCH[1]}
fi

if [ -n "${DOCKER_TAG}" ] && [ "${TRAVIS_GO_VERSION}" = "${GO_FOR_RELEASE}" ] && [ "${TRAVIS_BRANCH}" = "master" ]; then
	cat > ~/.dockercfg <<EOF
{
  "https://index.docker.io/v1/": {
    "auth": "${HUB_AUTH}",
    "email": "${HUB_EMAIL}"
  }
}
EOF
	echo "Pushing docker image to hub tagged as $DOCKER_TAG"
	docker build -t tsuru/api:${DOCKER_TAG} .
	docker push tsuru/api:${DOCKER_TAG}
else
	echo "No image to build"
fi
