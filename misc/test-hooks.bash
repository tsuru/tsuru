#!/bin/bash

# Copyright 2014 tsuru authors. All rights reserved.
# Use of this source code is governed by a BSD-style
# license that can be found in the LICENSE file.

status=0

echo tsuru receiving push | nc -l 5000 > /tmp/netcat.out &

mkdir /tmp/shell.git
ln -s $PWD/misc/git-hooks /tmp/shell.git/hooks
pushd /tmp/shell.git > /dev/null

echo -n "post-receive... "
out=`TSURU_HOST=http://127.0.0.1:5000 TSURU_TOKEN=000secret123 hooks/post-receive <<END
oldref newref refname
END`
gout=`echo $out | grep "tsuru receiving push"`

if [ $? != 0 ]
then
	echo "FAILURE: wrong output from post-receive command."
	echo "$out"
	status=1
fi

if [ "`tail -1 /tmp/netcat.out`" != "version=origin/master&commit=newref" ]
then
	echo "FAILURE: wrong request to the tsuru server."
	cat /tmp/netcat.out
	status=1
fi

header=`head -1 /tmp/netcat.out | sed -e "s/$(printf '\r')\$//"`
if [ "${header}" != "POST /apps/shell/repository/clone HTTP/1.1" ]
then
	echo "FAILURE: wrong request to the tsuru server."
	cat /tmp/netcat.out
	status=1
fi


if [ $status == 0 ]
then
	echo "PASS"
fi

popd > /dev/null
rm -rf /tmp/shell.git /tmp/netcat.out

kill %1

mongo tsuru > /dev/null 2>&1 <<END
db.dropDatabase();
END

exit $status
