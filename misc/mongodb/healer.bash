#!/bin/bash -u

# Copyright 2013 tsuru authors. All rights reserved.
# Use of this source code is governed by a BSD-style
# license that can be found in the LICENSE file.

# This script checks a set of collections and if one of them disappeared,
# downloads the backup and restores it.
#
# Usage:
#
#    ./mongo-healer.bash <bucket-path> <host> <database> <collection_1> [collection_2] ... [collection_n]

function usage() {
	echo "Usage:"
	echo
	echo "  $0 <bucket-path> <host> <database> <collection_1> [collection_2] ... [collection_n]"
	echo
	echo "      <bucket-path> is the path of the bucket where archives are stored."
	echo "                    It will be used if any restore is needed (example: s3://mybucket)."
	echo
	echo "      <host> is the database server to connect to (example: localhost)"
	echo
	echo "      <database> is the name of the database (example: tsuru)"
	echo
	echo "      [collection_1 .. collection_n] is the list of collections to watch."
	echo "                                     You must provide at least one collection."
	echo
}

# heal heals a collection. It takes the following parameters:
#
#   bucket-path ($1): path ot he bucket where archives are stored.
#   host ($2): database host to connect to.
#   database ($3): database to connect to.
#   collections ($4): array containing the name of collections to heal.
function heal() {
	healing=${@:4}
	file=`s3cmd ls $1 | grep mongodb-dump.tar.gz$ | tail -1 | awk '{print $4}'`
	if [ -z "$file" ]
	then
		echo "FATAL: no backups found!"
		exit 500
	fi
	echo "Downloading $file from S3..."
	s3cmd get $file dump.tar.gz
	tar -xzf dump.tar.gz
	echo
	for c in $healing
	do
		if [ -f dump/$3/$c.bson ]
		then
			echo "Restoring collection \"$c\"..."
			mongorestore dump/$3/$c.bson
		else
			echo "WARNING: backup for the collection \"$c\" does not exist. Skipping..."
		fi
	done
	rm -rf dump dump.tar.gz
}

if [ $# -lt 4 ]
then
	usage
	exit 1
fi

declare -a sick

watched=${@:4}
got=`mongo --host $2 --norc --quiet --eval "db.getCollectionNames().forEach(function(c) {print(c);});" $3`
i=0
for wc in $watched
do
	found=0
	for gc in $got
	do
		if [ $wc = $gc ]
		then
			found=1
			break
		fi
	done
	if [ $found = 0 ]
	then
		echo "collection \"$wc\" not found, marking as sick..."
		sick[$i]=$wc
		((i++))
	fi
done

if [ ${#sick[@]} -gt 0 ]
then
	echo
	coll=`printf ", %s" ${sick[@]}`
	echo "Healing collections: ${coll:2}."
	echo
	heal $1 $2 $3 ${sick[@]}
else
	echo "Everything is fine :-)"
	exit 0
fi
