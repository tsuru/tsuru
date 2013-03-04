#!/bin/bash -e

# Copyright 2013 tsuru authors. All rights reserved.
# Use of this source code is governed by a BSD-style
# license that can be found in the LICENSE file.

destination_dir="/tmp/dist-src"

function get_version {
	go build -o $1 github.com/globocom/tsuru/$2
	echo `./$1 version | awk '{print $3}' | sed -e 's/\.$//'`
	rm $1
}

function package {
	tar -czf $1 *
	shasum -a 256 $1
}

echo -n "Creating \"$destination_dir\" directory... "
mkdir -p $destination_dir
echo "ok"

echo -n "Determining crane version... "
crane_version=`get_version crane cmd/crane`
echo $crane_version

echo -n "Determining tsuru version... "
tsuru_version=`get_version tsuru cmd/tsuru/developer`
echo $tsuru_version

echo -n "Determining tsuru-admin version... "
admin_version=`get_version tsuru-admin cmd/tsuru/ops`
echo $admin_version

echo

package ${destination_dir}/crane-${crane_version}.tar.gz
package ${destination_dir}/tsuru-${tsuru_version}.tar.gz
package ${destination_dir}/tsuru-admin-${admin_version}.tar.gz
