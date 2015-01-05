#!/bin/sh -e

# Copyright 2015 tsuru authors. All rights reserved.
# Use of this source code is governed by a BSD-style
# license that can be found in the LICENSE file.

status=0

for f in `git ls-files | xargs grep "Copyright 201[234]" -l | grep -v check-license.sh`
do
	date=`git log -1 --format="%ad" --date=short -- $f`
	if [ `echo "$date" | grep ^2015` ]
	then
		echo $f $date
		status=1
	fi
done

exit $status
