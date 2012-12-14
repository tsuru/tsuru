#!/bin/sh -e

# Copyright 2012 tsuru authors. All rights reserved.
# Use of this source code is governed by a BSD-style
# license that can be found in the LICENSE file.

executable=/tmp/termtest
dir=`dirname $0`
go build -o $executable $dir/test.go
output=`echo '123' | $executable`
expected=`echo "Enter the password: \nThe password is \"123\"." | tr '\n' '\n'`
if [ "$output" != "$expected" ]
then
	echo "FAIL: expected \"$expected\""
	echo "      got \"$output\""
	exit 1
fi
rm $executable
echo "SUCCESS"
