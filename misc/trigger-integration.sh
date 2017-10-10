#!/bin/bash -e

# Copyright 2017 tsuru authors. All rights reserved.
# Use of this source code is governed by a BSD-style
# license that can be found in the LICENSE file.

TSURUVERSION=""

if [ -n "${TRAVIS_TAG}" ]; then
    TSURUVERSION="v${TRAVIS_TAG}"
elif [[ "${TRAVIS_BRANCH}" =~ release-.+ ]]; then
    TSURUVERSION="${TRAVIS_BRANCH}"
fi

if [ -n "${TSURUVERSION}" ]; then
    echo "triggering ec2 integration test for $TSURUVERSION"
    curl --header "Content-Type: application/json" --data '{"build_parameters": {"TSURUVERSION": "'"$TSURUVERSION"'"}}' \
    --request POST https://circleci.com/api/v1.1/project/github/tsuru/integration_ec2/tree/master?circle-token=$CIRCLE_EC2_TOKEN

    echo "triggering gce integration test for $TSURUVERSION"
    curl --header "Content-Type: application/json" --data '{"build_parameters": {"TSURUVERSION": "'"$TSURUVERSION"'"}}' \
    --request POST https://circleci.com/api/v1.1/project/github/tsuru/integration_gce/tree/master?circle-token=$CIRCLE_GCE_TOKEN
fi
