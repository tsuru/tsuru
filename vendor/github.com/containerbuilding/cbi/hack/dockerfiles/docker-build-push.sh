#!/bin/sh
set -e -x

if [ "$#" -lt 1 ]; then
    echo "Usage: $0 BUILDFLAGS"
    exit 1
fi

if [ -z "${DBP_DOCKER_BINARY}" ]; then
    echo "DBP_DOCKER_BINARY needs to be set (string)"
    exit 1
fi


if [ -z "${DBP_IMAGE_NAME}" ]; then
    echo "DBP_IMAGE_NAME needs to be set (string)"
    exit 1
fi

if [ -z "${DBP_PUSH}" ]; then
    echo "DBP_PUSH needs to be set (0 or 1)"
    exit 1
fi

if [ -z "${DBP_DIALECT}" ]; then
    echo "DBP_DIALECT needs to be set (docker or buildah)"
    exit 1
fi

case ${DBP_DIALECT} in
    docker )
        ${DBP_DOCKER_BINARY} build -t ${DBP_IMAGE_NAME} $@ ;;
    buildah )
        ${DBP_DOCKER_BINARY} bud -t ${DBP_IMAGE_NAME} $@ ;;
    *)
        echo "Unsupported dialect: ${DBP_DIALECT}"
        exit 1
esac

if [ "${DBP_PUSH}" = 1 ]; then
    case ${DBP_DIALECT} in
        docker )
            ${DBP_DOCKER_BINARY} push ${DBP_IMAGE_NAME} ;;
        buildah )
            ${DBP_DOCKER_BINARY} push ${DBP_IMAGE_NAME} docker://${DBP_IMAGE_NAME} ;;
        *)
            echo "Unsupported dialect: ${DBP_DIALECT}"
            exit 1
    esac
fi
