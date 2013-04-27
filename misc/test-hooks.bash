#!/bin/bash

# Copyright 2013 tsuru authors. All rights reserved.
# Use of this source code is governed by a BSD-style
# license that can be found in the LICENSE file.

# TODO(fss): this script is a hack. I should rewrite it :)

echo "RUNNING GIT-HOOKS TESTS"
mongo tsuru > /dev/null 2>.mongo.err <<END
var today = new Date();
var tomorrow = new Date(today.getTime() + 24 * 60 * 60 * 1000);
db.users.insert({"email": "test@shellscript", "password": "irrelevant"});
db.teams.insert({"_id": "test", "users": ["test@shellscript"]})
db.tokens.insert({"token": "000secret123", "validuntil": tomorrow, "useremail": "test@shellscript"})
var app1 = {
	"name": "shell",
	"framework": "bash",
	"ip": "10.10.10.10",
	"cname": "myapp.com",
	"units": [
		{
			"name": "shell/0",
			"type": "bash",
			"instanceid": "i-0800",
			"ip": "10.10.10.10",
			"state": "started"
		}
	],
	"teams": ["test"]
};
var app2 = {
	"name": "xeu",
	"framework": "bash",
	"ip": "10.10.10.11",
	"cname": "myapp.com",
	"units": [
		{
			"name": "xeu/0",
			"type": "bash",
			"instanceid": "i-0801",
			"ip": "10.10.10.11",
			"state": "pending"
		}
	],
	"teams": ["test"]
};
db.apps.insert(app1);
db.apps.insert(app2);
END

if [ $? != 0 ]
then
	echo "FAILURE: failed to insert data into the database"
	cat .mongo.err
	rm .mongo.err
	exit 7
fi
rm .mongo.err

go build -o tsr ./cmd/tsr
./tsr api --config ./etc/tsuru.conf > .api.out 2>&1 &

if [ $? != 0 ]
then
	echo "FAILURE: failed to build api server"
	cat .api.out
	rm .api.out
	rm tsr
	exit 1
fi
rm .api.out

sleep 1
nc -z localhost 8080 > /dev/null

status=0

mkdir /tmp/shell.git
ln -s $PWD/misc/git-hooks /tmp/shell.git/hooks
pushd /tmp/shell.git > /dev/null

echo -n "pre-receive on available app... "
out=`TSURU_HOST=http://127.0.0.1:8080 TSURU_TOKEN=000secret123 hooks/pre-receive`

if [ $? = 0 ]
then
	echo "PASS"
else
	echo "FAILURE: expected 0 status from pre-receive on shell app."
	echo "$out"
	status=1
fi

echo -n "post-receive... "
out=`TSURU_HOST=http://127.0.0.1:8080 TSURU_TOKEN=000secret123 hooks/post-receive`
gout=`echo $out | grep "Tsuru receiving push"`

if [ $? = 0 ]
then
	echo "PASS"
else
	echo "FAILURE: wrong output from post-receive command."
	echo "$out"
	status=1
fi

popd > /dev/null
rm -rf /tmp/shell.git

echo -n "pre-receive on unavailable app... "

mkdir /tmp/xeu.git
ln -s $PWD/misc/git-hooks /tmp/xeu.git/hooks
pushd /tmp/xeu.git > /dev/null

TSURU_HOST=http://127.0.0.1:8080 TSURU_TOKEN=000secret123 hooks/pre-receive > .pre-receive.out 2>&1

if [ $? != 0 ]
then
	echo "PASS"
else
	echo "FAILURE: got wrong status from pre-receive hook for unavailable app"
	cat .pre-receive.out
	status=1
fi
rm .pre-receive.out

popd > /dev/null
rm -rf /tmp/xeu.git

kill %1

mongo tsuru > /dev/null 2>&1 <<END
db.dropDatabase();
END

rm tsr

exit $status
