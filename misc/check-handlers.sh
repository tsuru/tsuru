#!/bin/bash

set -e

function check_go {
    set +e
    version=$(go version | grep -o 'go1.5')
    if [ "${GO15VENDOREXPERIMENT}" != "0" ] && [ -n "${version}" ]; then
        echo "Skipping handlers checking, it requires Go 1.6 or higher. Please upgrade Go or disable GO15VENDOREXPERIMENT."
        exit 0
    fi
    set -e
}

function missing_handlers {
    go get -u github.com/tsuru/tsuru-api-docs

    allhandlers=$(tsuru-api-docs -search . | sort)

    okhandlers1=$(tsuru-api-docs -search 'permission.Check\(')
    okhandlers2=$(tsuru-api-docs -search 'permission.ContextsForPermission\(')
    okhandlers3=$(tsuru-api-docs -search 'permission.CheckFromPermList\(')

    okhandlers=$(cat <(echo "$okhandlers1") <(echo "$okhandlers2") <(echo "$okhandlers3") | sort | uniq)

    ignored=$(cat <<EOF
github.com/tsuru/tsuru/api.authScheme
github.com/tsuru/tsuru/api.healthcheck
github.com/tsuru/tsuru/api.index
github.com/tsuru/tsuru/api.info
github.com/tsuru/tsuru/api.resetPassword
github.com/tsuru/tsuru/api.samlCallbackLogin
github.com/tsuru/tsuru/api.samlMetadata
github.com/tsuru/tsuru/api.addKeyToUser
github.com/tsuru/tsuru/api.listPlans
github.com/tsuru/tsuru/api.login
github.com/tsuru/tsuru/api.logout
github.com/tsuru/tsuru/api.changePassword
github.com/tsuru/tsuru/api.userInfo
github.com/tsuru/tsuru/api.serviceInfo
github.com/tsuru/tsuru/api.serviceInstances
github.com/tsuru/tsuru/api.listKeys
github.com/tsuru/tsuru/api.listUsers
github.com/tsuru/tsuru/api.removeKeyFromUser
github.com/tsuru/tsuru/api.setNodeStatus
github.com/tsuru/tsuru/api.kindList
github.com/tsuru/tsuru/api.eventList
github.com/tsuru/tsuru/api.eventInfo
github.com/tsuru/tsuru/api.eventCancel
github.com/tsuru/tsuru/api.listNodesHandler
github.com/tsuru/tsuru/api.nodeContainerInfo
github.com/tsuru/tsuru/api.nodeContainerList
github.com/tsuru/tsuru/api.nodeHealingRead
github.com/tsuru/tsuru/api.tokenList
github.com/tsuru/tsuru/provision/docker.bsConfigGetHandler
github.com/tsuru/tsuru/provision/docker.logsConfigGetHandler
github.com/tsuru/tsuru/provision/docker.bsEnvSetHandler
github.com/tsuru/tsuru/provision/docker.bsUpgradeHandler
EOF
    )
    ignored=$(echo "$ignored" | sort)

    allhandlers=$(comm -23 <(echo "$allhandlers") <(echo "$ignored"))
    allhandlers=$(comm -23 <(echo "$allhandlers") <(echo "$okhandlers"))

    if [ -n "$okhandlers" ]; then
        len=$(echo "$okhandlers" | wc -l)
        echo "OK handlers: $len"$'\n'"$okhandlers"
    fi

    if [ -n "$allhandlers" ]; then
        len=$(echo "$allhandlers" | wc -l)
        echo "Misssing handlers: $len"$'\n'"$allhandlers"
        exit 1
    fi
}

function extra_perms {
    allperms=$(cat permission/permitems.go | egrep -o ".*=" | egrep -o "Perm[a-zA-Z]+" | grep -v PermAll)
    newperms=""
    for p in $allperms; do
        count=$(grep $p permission/permitems.go | wc -l)
        if [ $count == "1" ]; then
            newperms="$p"$'\n'"$newperms"
        fi
    done
    fail=
    for p in $newperms; do
        uses=$((grep -R --exclude-dir "permission" --exclude-dir ".git" --exclude "*_test.go" $p * | grep -v "Binary file") || true)
        if [[ -z $uses ]]; then
            fail=1
            echo "Unused permission $p"
        fi
    done
    test -z "$fail"
}

check_go
missing_handlers
extra_perms
