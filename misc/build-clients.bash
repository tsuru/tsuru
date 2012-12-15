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

# TODO: handle version of commands.

destination_dir="dist-cmd"

echo -n "Creating \"$destination_dir\" directory... "
mkdir -p $destination_dir
echo "ok"

targets="darwin_amd64 linux_386 linux_amd64"

for target in $targets
do
	echo -n "Building crane for $target... "
	os=`echo $target | cut -d '_' -f1`
	arch=`echo $target | cut -d '_' -f2`
	GOOS=$os GOARCH=$arg CGO_ENABLED=0 go build -o $destination_dir/crane github.com/globocom/tsuru/cmd/crane
	tar -czf $destination_dir/crane-$os-$arch.tar.gz $destination_dir/crane
	rm $destination_dir/crane
	echo "ok"
done

for target in $targets
do
	echo -n "Building tsuru for $target... "
	os=`echo $target | cut -d '_' -f1`
	arch=`echo $target | cut -d '_' -f2`
	GOOS=$os GOARCH=$arg CGO_ENABLED=0 go build -o $destination_dir/tsuru github.com/globocom/tsuru/cmd/tsuru/developer
	tar -czf $destination_dir/tsuru-$os-$arch.tar.gz $destination_dir/tsuru
	rm $destination_dir/tsuru
	echo "ok"
done

for target in $targets
do
	echo -n "Building tsuru-admin for $target... "
	os=`echo $target | cut -d '_' -f1`
	arch=`echo $target | cut -d '_' -f2`
	GOOS=$os GOARCH=$arg CGO_ENABLED=0 go build -o $destination_dir/tsuru-admin github.com/globocom/tsuru/cmd/tsuru/ops
	tar -czf $destination_dir/tsuru-admin-$os-$arch.tar.gz $destination_dir/tsuru-admin
	rm $destination_dir/tsuru-admin
	echo "ok"
done
