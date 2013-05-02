#!/bin/bash -e

# Copyright 2013 tsuru authors. All rights reserved.
# Use of this source code is governed by a BSD-style
# license that can be found in the LICENSE file.

# This script is used to install tsuru with lxc provisioner.

function configure_tsuru() {
    echo "Configuring tsuru"
    [ -d /etc/tsuru ] || sudo mkdir /etc/tsuru
    sudo curl -sL https://raw.github.com/globocom/tsuru/master/etc/tsuru-docker.conf -o /etc/tsuru/tsuru.conf
}

function install_docker() {
    sudo apt-get install lxc wget bsdtar curl
    # are you on AWS? if so, uncomment the line below
    # sudo apt-get install linux-image-extra-`uname -r`
    wget http://get.docker.io/builds/$(uname -s)/$(uname -m)/docker-master.tgz
    tar -xf docker-master.tgz
    cd docker-master
    sudo cp docker /usr/local/bin
    # runs docker daemon, it must be running in order to tsuru work
    sudo docker -d &
}

function start_tsuru() {
    echo "starting mongodb"
    sudo service mongodb start
    echo "starting beanstalkd"
    sudo service beanstalkd start
    echo "starting tsuru-collector"
    tsr collector &
    echo "starting tsuru-api"
    sudo tsr api &
}

function main() {
    source tsuru-setup.bash
    source gandalf-setup.bash
    install_docker
    configure_tsuru
    start_tsuru
}

main
