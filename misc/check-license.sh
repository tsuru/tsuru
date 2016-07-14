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

curYear=$(date +"%Y")

tofix=()
for f in `git ls-files | grep -v vendor/ | grep -v check-license.sh | xargs -I{} bash -c "(egrep -o \"Copyright [0-9]+\" {} | grep -v ${curYear} >/dev/null) && echo {}"`
do
	date=`git log -1 --format="%ad" --date=short -- $f`
	if [ `echo "$date" | grep "^${curYear}"` ]
	then
        tofix+=($f)
		echo $f $date
		status=1
	fi
done

case $1 in
    "-f" | "--fix")
        for f in ${tofix[@]}; do
            sed -E -i "" "s/Copyright [0-9]+/Copyright ${curYear}/g" $f
        done
        exit 0
        ;;
esac

exit $status
