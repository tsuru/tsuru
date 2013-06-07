#!/bin/bash

# Copyright 2013 tsuru authors. All rights reserved.
# Use of this source code is governed by a BSD-style
# license that can be found in the LICENSE file.

# This script runs --repair and then starts mongodb.
#
# Usage:
#
#    ./start.bash <db-path>

mongod --repair --syslog --dbpath $1
exec mongod --syslog --dbpath $1
