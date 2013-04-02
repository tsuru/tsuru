#!/bin/sh -e

# Copyright 2013 tsuru authors. All rights reserved.
# Use of this source code is governed by a BSD-style
# license that can be found in the LICENSE file.

for f in `grep "Copyright 2012" -r . -l`
do
	date=`git log -1 --format="%ad" --date=short -- $f`
	if [ `echo "$date" | grep ^2013` ]
	then
		echo $f $date
	fi
done
