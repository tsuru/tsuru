#!/bin/bash

# Copyright 2016 tsuru authors. All rights reserved.
# Use of this source code is governed by a BSD-style
# license that can be found in the LICENSE file.

dirs=$(go list -f '{{.Dir}}/*.go' ./... | grep -v vendor)

status=0
out=`gofmt -s -l $dirs`
if [ "${out}" != "" ]
then
    echo "ERROR: there are files that need to be formatted with gofmt"
    echo
    echo "Files:"
    for file in $out
    do
        echo "- ${file}"
    done
    echo
    status=1
fi

go get golang.org/x/tools/cmd/goimports
out=`goimports -l $dirs`

if [ "${out}" != "" ]
then
    echo "ERROR: there are files that need to be formatted with goimports"
    echo
    echo "Files:"
    for file in $out
    do
        echo "- ${file}"
    done
    status=1
fi

out=`go tool vet -shadow -all $dirs 2>&1`
if [ "${out}" != "" ]
then
    echo "ERROR: go vet failures:"
    echo
    cat <<END
${out}
END
    status=1
fi

exit $status
