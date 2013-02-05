#!/bin/bash -e

# Copyright 2013 tsuru authors. All rights reserved.
# Use of this source code is governed by a BSD-style
# license that can be found in the LICENSE file.

status=0
out=`gofmt -s -l .`
if [ "${out}" != "" ]
then
	echo "ERROR: there are files that need to be formatted with gofmt"
	echo
	echo "Files:"
	for file in $out
	do
	    echo "- ${file}"
	done
	status=1
fi

path=$PWD
for p in `go list ./...`
do
	pushd $GOPATH/src/$p > /dev/null
	go vet >> $path/.vet
	popd > /dev/null
done

if [ -f .vet ]
then
	out=`cat .vet`
	if [ "${out}" != "" ]
	then
		echo "ERROR: go vet failures:"
		echo
		cat <<END
	${out}
END
		status=1
	fi
	rm .vet
fi

exit $status
