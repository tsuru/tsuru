#!/bin/bash -e

go get golang.org/x/tools/cmd/oracle

pos=$(cat ./api/handler.go  | grep -ob "fn authorizationRequiredHandler" | egrep -o "^\d+")
handlers1=$(oracle -pos=./api/handler.go:#$pos pointsto github.com/tsuru/tsuru/cmd/tsurud | tail +2 | awk '{print $2}')

pos=$(cat ./api/handler.go  | grep -ob "fn AdminRequiredHandler" | egrep -o "^\d+")
handlers2=$(oracle -pos=./api/handler.go:#$pos pointsto github.com/tsuru/tsuru/cmd/tsurud | tail +2 | awk '{print $2}')

allhandlers=$(echo "$handlers1"$'\n'"$handlers2" | sort)

pos=$(($(cat ./permission/permission.go | grep -ob "func Check" | egrep -o "^\d+")+5))
okhandlers=$(oracle -pos=./permission/permission.go:#$pos callers github.com/tsuru/tsuru/cmd/tsurud | tail +2 | egrep -o " github.*" | awk '{print $1}' | sort)

ignored=$(cat <<EOF
EOF
)
ignored=$(echo "$ignored" | sort)

allhandlers=$(comm -23 <(echo "$allhandlers") <(echo "$ignored"))
missing=$(comm -23 <(echo "$allhandlers") <(echo "$okhandlers"))

if [ -n "$missing" ]; then
    echo "Misssing handlers:"$'\n'"$missing"
    exit 1
fi
