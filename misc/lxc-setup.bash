#!/bin/bash

# Copyright 2013 tsuru authors. All rights reserved.
# Use of this source code is governed by a BSD-style
# license that can be found in the LICENSE file.

# This script is used to install tsuru with lxc provisioner.

echo "installing lxc"
sudo apt-get install lxc -y

echo "installing beanstalkd"
sudo apt-get install -y beanstalkd

echo "installing mongodb"
sudo apt-key adv --keyserver keyserver.ubuntu.com --recv 7F0CEB10
sudo bash -c 'echo "deb http://downloads-distro.mongodb.org/repo/ubuntu-upstart dist 10gen" > /etc/apt/sources.list.d/10gen.list'
sudo apt-get update
sudo apt-get install mongodb-10gen -y

echo "installing tsuru-collector"
curl -sL https://s3.amazonaws.com/tsuru/dist-server/tsuru-collector.tar.gz | sudo tar -xz -C /usr/bin

echo "installing tsuru-api"
curl -sL https://s3.amazonaws.com/tsuru/dist-server/tsuru-api.tar.gz | sudo tar -xz -C /usr/bin

echo "configuring tsuru"
sudo mkdir /etc/tsuru
curl -sL https://raw.github.com/globocom/tsuru/master/etc/tsuru-lxc.conf -o /etc/tsuru/tsuru.conf

echo "starting mongodb"
sudo service mongodb start

echo "starting beanstalkd"
sudo service beanstalkd start

echo "starting tsuru-collector"
collector &

echo "starting tsuru-api"
api &
