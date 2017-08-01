#!/bin/bash -e

# Copyright 2017 tsuru authors. All rights reserved.
# Use of this source code is governed by a BSD-style
# license that can be found in the LICENSE file.

if [ -n "$TRAVIS_TAG" ]
then 
    echo "triggering ec2 integration test for $TRAVIS_TAG"
    curl --header "Content-Type: application/json" --data '{"build_parameters": {"TSURUVERSION": "'"v$TRAVIS_TAG"'"}}' \
    --request POST https://circleci.com/api/v1.1/project/github/tsuru/integration_ec2/tree/master?circle-token=$CIRCLE_EC2_TOKEN
fi