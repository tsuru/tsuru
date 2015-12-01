#!/bin/bash -e

function missing_handlers {
    go get golang.org/x/tools/cmd/oracle

    pos=$(cat ./api/handler.go  | grep -ob "fn AuthorizationRequiredHandler" | egrep -o "^[0-9]+")
    allhandlers=$(oracle -pos=./api/handler.go:#$pos pointsto github.com/tsuru/tsuru/cmd/tsurud | tail -n+2 | awk '{print $2}' | sort)

    pos=$(($(cat ./permission/permission.go | grep -ob "func Check(" | egrep -o "^[0-9]+")+5))
    okhandlers1=$(oracle -pos=./permission/permission.go:#$pos callers github.com/tsuru/tsuru/cmd/tsurud | tail -n+2 | egrep -o " github.*" | awk '{print $1}' | sort)

    pos=$(($(cat ./permission/permission.go | grep -ob "func ContextsForPermission" | egrep -o "^[0-9]+")+5))
    okhandlers2=$(oracle -pos=./permission/permission.go:#$pos callers github.com/tsuru/tsuru/cmd/tsurud | tail -n+2 | egrep -o " github.*" | awk '{print $1}' | sort)

    pos=$(($(cat ./permission/permission.go | grep -ob "func CheckFromPermList" | egrep -o "^[0-9]+")+5))
    okhandlers3=$(oracle -pos=./permission/permission.go:#$pos callers github.com/tsuru/tsuru/cmd/tsurud | tail -n+2 | egrep -o " github.*" | awk '{print $1}' | sort)

    okhandlers=$(cat <(echo "$okhandlers1") <(echo "$okhandlers2") <(echo "$okhandlers3") | sort | uniq)

    ignored=$(cat <<EOF
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
github.com/tsuru/tsuru/api.setUnitsStatus
github.com/tsuru/tsuru/provision/docker.bsConfigGetHandler
github.com/tsuru/tsuru/provision/docker.listNodesHandler
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
        uses=$(grep -R --exclude-dir "permission" --exclude-dir ".git" --exclude "*_test.go" $p * || true)
        if [[ -z $uses ]]; then
            fail=1
            echo "Unused permission $p"
        fi
    done
    test -z "$fail"
}

missing_handlers
extra_perms
