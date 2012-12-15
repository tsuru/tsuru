#!/bin/bash -e

# Copyright 2012 tsuru authors. All rights reserved.
# Use of this source code is governed by a BSD-style
# license that can be found in the LICENSE file.

# This script is used to build tsuru, tsuru-admin and crane on the following
# platforms:
#
#     - darwin_amd64
#     - linux_386
#     - linux_amd64

destination_dir="dist-cmd"

function get_version {
	GOOS=`go env GOHOSTOS` GOARCH=`go env GOHOSTARCH` CGO_ENABLED=0 go build -o $destination_dir/$1 \
		github.com/globocom/tsuru/$2
	echo `$destination_dir/$1 version | awk '{print $3}' | sed -e 's/\.$//'`
}

function build_and_package {
	echo -n "Building $2 $4 for $1... "
	os=`echo $1 | cut -d '_' -f1`
	arch=`echo $1 | cut -d '_' -f2`
 	GOOS=$os GOARCH=$arch CGO_ENABLED=0 go build -o $destination_dir/$2 github.com/globocom/tsuru/$3
	tar -czf $destination_dir/$2-$os-$arch-$4.tar.gz $destination_dir/$2
	rm $destination_dir/$2
	echo "ok"
}

echo -n "Creating \"$destination_dir\" directory... "
mkdir -p $destination_dir
echo "ok"

targets="darwin_amd64 linux_386 linux_amd64"

echo -n "Determining crane version... "
crane_version=`get_version crane cmd/crane`
echo $crane_version

echo -n "Determining tsuru version... "
tsuru_version=`get_version tsuru cmd/tsuru/developer`
echo $tsuru_version

echo -n "Determining tsuru-admin version... "
tsuru_admin_version=`get_version tsuru-admin cmd/tsuru/ops`
echo $tsuru_admin_version

for target in $targets
do
	build_and_package $target crane cmd/crane $crane_version
done

for target in $targets
do
	build_and_package $target tsuru cmd/tsuru/developer $tsuru_version
done

for target in $targets
do
	build_and_package $target tsuru-admin cmd/tsuru/ops $tsuru_admin_version
done
