#!/bin/bash -e

# Copyright 2013 tsuru authors. All rights reserved.
# Use of this source code is governed by a BSD-style
# license that can be found in the LICENSE file.

# This script is used to build tsuru, tsuru-admin and crane in the specified
# platform.
#
# Usage:
#
#   % build-clients.bash <os>_<arch>

if [ $# -lt 1 ]
then
	echo "Usage: "
	echo
	echo "  % $0 <os>_<arch>"
	exit 7
fi

destination_dir="dist-cmd"

function build_and_package {
	echo -n "Building $2 for $1... "
	os=`echo $1 | cut -d '_' -f1`
	arch=`echo $1 | cut -d '_' -f2`
 	GOOS=$os GOARCH=$arch CGO_ENABLED=0 go build -o $destination_dir/$2 github.com/globocom/tsuru/$3
	tar -C $destination_dir -czf $destination_dir/$2-$os-$arch.tar.gz $2
	rm $destination_dir/$2
	echo "ok"
}

echo -n "Creating \"$destination_dir\" directory... "
mkdir -p $destination_dir
echo "ok"

build_and_package $1 crane cmd/crane
build_and_package $1 tsuru cmd/tsuru
build_and_package $1 tsuru-admin cmd/tsuru-admin
