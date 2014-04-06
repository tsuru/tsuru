#!/bin/bash -eu

# Copyright 2014 tsuru authors. All rights reserved.
# Use of this source code is governed by a BSD-style
# license that can be found in the LICENSE file.

destination_dir="dist-server"
destination_dir=${DESTINATION_DIR:-$destination_dir}

echo -n "Creating \"$destination_dir\" directory... "
mkdir -p $destination_dir
echo "ok"

echo -n "Downloading dependencies... "
go get -u -d github.com/tsuru/tsuru/cmd/tsr
echo "ok"

pushd $GOPATH/src/github.com/tsuru/tsuru

echo -n "Checking out $REVISION... "
git checkout $REVISION
echo "ok"

echo -n "Restoring dependencies version... "
godep restore ./...
echo "ok"

popd

BUILD_FLAGS="-a -o"
POSTFIX=""

if [ $PPROF = true ]
then
	BUILD_FLAGS="-tags pprof $BUILD_FLAGS"
	POSTFIX="-pprof"
fi

echo "Building tsr-${REVISION}... "
godep go build $BUILD_FLAGS $destination_dir/tsr github.com/tsuru/tsuru/cmd/tsr
tar -C $destination_dir -czf $destination_dir/tsr-${REVISION}${POSTFIX}.tar.gz tsr
rm $destination_dir/tsr
