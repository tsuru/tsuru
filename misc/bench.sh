#!/bin/bash
IFS=

if [[ "$BENCH_FORM" == "" ]]; then
    echo "BENCH_FORM environment required"
    exit 0
fi

benchLines="$(go list -f '{{.Dir}}' ./... | xargs -I{} bash -c 'pushd {}; go test -check.b -check.bmem 2>&1; popd' | egrep -o 'Benchmark.*' | tee /dev/stderr)"
baseCurl="curl -o /dev/null -sS "$BENCH_FORM" -H 'content-type: application/x-www-form-urlencoded' --data"
commit=$(git log --pretty='format:%H' -1) 
while read l; do
    cmd=$(echo $l | awk '{print "'$baseCurl' '\''entry.559666149="$1"&entry.319255563='$commit'&entry.1798633669="$2"&entry.2023870129="$3"&entry.328660811="$5"&entry.1525448807="$7"'\''"}')
    bash -c "$cmd"
done < <(echo $benchLines)

