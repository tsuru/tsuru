#!/bin/bash

set -e

# sort(1) output below is consumed by comm(1), which requires both inputs to
# share a collation order. Locales such as en_US.UTF-8 make comm reject the
# input with "file 1 is not in sorted order".
export LC_ALL=C

ignored=$(
  cat <<EOF
github.com/tsuru/tsuru/api.registerUnit
github.com/tsuru/tsuru/api.setUnitStatus
github.com/tsuru/tsuru/api.setNodeStatus
github.com/tsuru/tsuru/api.addLog
github.com/tsuru/tsuru/api.logout
github.com/tsuru/tsuru/api.login
github.com/tsuru/tsuru/api.forceDeleteLock
github.com/tsuru/tsuru/api.diffDeploy
github.com/tsuru/tsuru/api.swap
EOF
)
ignored=$(echo "$ignored" | sort)

go install github.com/tsuru/tsuru-api-docs@v0.0.1
badhandlers=$(tsuru-api-docs --no-method GET --no-search "event\." | sort)
badhandlers=$(comm -23 <(echo "$badhandlers") <(echo "$ignored"))

if [ -n "$badhandlers" ]; then
  len=$(echo "$badhandlers" | wc -l)
  echo "Misssing event handlers: $len"$'\n'"$badhandlers"
  exit 1
fi
