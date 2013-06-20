#!/bin/sh

# Copyright 2013 tsuru authors. All rights reserved.
# Use of this source code is governed by a BSD-style
# license that can be found in the LICENSE file.

status=0

for email in `git log --format=%ae | sort | uniq | grep -v globo@Mac-2.local`
do
	grep -q $email CONTRIBUTORS
	if [ $? != 0  ]
	then
		echo "ERROR: $email is not in the CONTRIBUTORS file."
		status=1
	fi
done

exit $status
