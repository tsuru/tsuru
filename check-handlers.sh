#!/bin/bash -e

go get golang.org/x/tools/cmd/oracle

pos=$(cat ./api/handler.go  | grep -ob "fn authorizationRequiredHandler" | egrep -o "^[0-9]+")
handlers1=$(oracle -pos=./api/handler.go:#$pos pointsto github.com/tsuru/tsuru/cmd/tsurud | tail -n+2 | awk '{print $2}')

pos=$(cat ./api/handler.go  | grep -ob "fn AdminRequiredHandler" | egrep -o "^[0-9]+")
handlers2=$(oracle -pos=./api/handler.go:#$pos pointsto github.com/tsuru/tsuru/cmd/tsurud | tail -n+2 | awk '{print $2}')

allhandlers=$(echo "$handlers1"$'\n'"$handlers2" | sort)

pos=$(($(cat ./permission/permission.go | grep -ob "func Check(" | egrep -o "^[0-9]+")+5))
okhandlers1=$(oracle -pos=./permission/permission.go:#$pos callers github.com/tsuru/tsuru/cmd/tsurud | tail -n+2 | egrep -o " github.*" | awk '{print $1}' | sort)

pos=$(($(cat ./permission/permission.go | grep -ob "func ContextsForPermission" | egrep -o "^[0-9]+")+5))
okhandlers2=$(oracle -pos=./permission/permission.go:#$pos callers github.com/tsuru/tsuru/cmd/tsurud | tail -n+2 | egrep -o " github.*" | awk '{print $1}' | sort)

okhandlers=$(cat <(echo "$okhandlers1") <(echo "$okhandlers2") | sort | uniq)

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
