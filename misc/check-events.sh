#!/bin/bash

set -e

ignored=$(cat <<EOF
github.com/tsuru/tsuru/api.registerUnit
github.com/tsuru/tsuru/api.setUnitStatus
github.com/tsuru/tsuru/api.setNodeStatus
github.com/tsuru/tsuru/api.addLog
github.com/tsuru/tsuru/api.logout
github.com/tsuru/tsuru/api.login
github.com/tsuru/tsuru/api.webLogin
github.com/tsuru/tsuru/api.samlCallbackLogin
github.com/tsuru/tsuru/api.forceDeleteLock
github.com/tsuru/tsuru/api.diffDeploy
EOF
)
ignored=$(echo "$ignored" | sort)

go install github.com/tsuru/tsuru-api-docs@latest
badhandlers=$(tsuru-api-docs --no-method GET --no-search "event\." | sort)
badhandlers=$(comm -23 <(echo "$badhandlers") <(echo "$ignored"))

if [ -n "$badhandlers" ]; then
    len=$(echo "$badhandlers" | wc -l)
    echo "Misssing event handlers: $len"$'\n'"$badhandlers"
    exit 1
fi
