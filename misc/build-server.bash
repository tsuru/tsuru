#!/bin/bash -eu

# Copyright 2013 tsuru authors. All rights reserved.
# Use of this source code is governed by a BSD-style
# license that can be found in the LICENSE file.

# This script is used to build components from tsuru server (webserver and
# collector).

destination_dir="dist-server"

function build_and_package {
	echo "Building $1... "
 	go build -o $destination_dir/$1 github.com/globocom/tsuru/$1
	tar -C $destination_dir -czf $destination_dir/tsuru-$1.tar.gz $1
	rm $destination_dir/$1
}

echo -n "Creating \"$destination_dir\" directory... "
mkdir -p $destination_dir
echo "ok"

build_and_package collector
build_and_package api
