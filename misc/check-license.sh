#!/bin/bash -e

# Copyright 2016 tsuru authors. All rights reserved.
# Use of this source code is governed by a BSD-style
# license that can be found in the LICENSE file.

status=0
for f in `git ls-files | xargs grep -L "Copyright" | grep ".go" | grep -v vendor/`
do
    echo $f
    status=1
done

if [ $status != 0 ]
then
   exit $status
fi

tofix=
addallyears=
while [ "${1-}" != "" ]; do
    case $1 in
        "-f" | "--fix")
            tofix=true
            ;;
        "--all")
            addallyears=true
            ;;
    esac
    shift
done

oldIFS=$IFS
IFS=$(echo -en "\n\b")

function join_space { 
    IFS=" "
    echo "$*"
}

for f in $(git ls-files | grep -v vendor/ | grep -v check-license.sh | xargs -I{} bash -c '(egrep -Ho "Copyright [0-9 ]+" {})')
do
    IFS=":" read file copyright <<< "$f"
    IFS=" " read copy year <<< "$copyright"
    if [ -z $addallyears ]; then
        expectedYears=`git log --diff-filter=A --follow --format=%ad --date=format:%Y -1 -- $file`
    else
        expectedYears=$(join_space $(git log --follow --format=%ad --date=format:%Y -- $file | sort | uniq))
    fi
    if [[ $year != $expectedYears ]];
    then
        echo "$file - Copyright $year, created: $expectedYears"
        if [ -z "$tofix" ]; then
            status=1
        else
            sed -E -i "" "s/Copyright [0-9 ]+/Copyright ${expectedYears} /g" $file
        fi
    fi
done

exit $status
