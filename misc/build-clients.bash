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

function build_and_package {
	echo -n "Building $2 for $1... "
	os=`echo $1 | cut -d '_' -f1`
	arch=`echo $1 | cut -d '_' -f2`
 	GOOS=$os GOARCH=$arch CGO_ENABLED=0 go build -o $destination_dir/$2 github.com/globocom/tsuru/$3
	tar -czf $destination_dir/$2-$os-$arch.tar.gz $destination_dir/$2
	rm $destination_dir/$2
	echo "ok"
}

echo -n "Creating \"$destination_dir\" directory... "
mkdir -p $destination_dir
echo "ok"

targets="darwin_amd64 linux_386 linux_amd64"

for target in $targets
do
	build_and_package $target crane cmd/crane
done

for target in $targets
do
	build_and_package $target tsuru cmd/tsuru/developer
done

for target in $targets
do
	build_and_package $target tsuru-admin cmd/tsuru/ops
done
