#!/bin/bash -e

# Copyright 2013 tsuru authors. All rights reserved.
# Use of this source code is governed by a BSD-style
# license that can be found in the LICENSE file.

# This script is used to backup MongoDB and send it to a bucket in S3. To use
# this script, you need to install and configure s3cmd before.
#
# Usage:
#
#    ./backup-mongodb.bash <bucket-path> <host> <database>

filename="`date +%Y-%m-%d-%H-%M-%S`-mongodb-dump.tar.gz"

if [ $# -lt 3 ]
then
	echo "Usage:"
	echo
	echo "  $0 <bucket-path> <host> <database>"
	exit 1
fi

echo "Dumping ${3} from ${2} and saving in the bucket ${1}..."
mongodump -h $2 -d $3
tar -czf ${filename} dump
rm -rf dump
s3cmd put ${filename} ${1}
rm -f ${filename}
