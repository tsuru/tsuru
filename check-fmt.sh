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

exit $status
