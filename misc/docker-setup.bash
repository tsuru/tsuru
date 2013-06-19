#!/bin/bash -e

# Copyright 2013 tsuru authors. All rights reserved.
# Use of this source code is governed by a BSD-style
# license that can be found in the LICENSE file.

# This script is used to install tsuru with docker provisioner.

function configure_tsuru() {
    echo "Configuring tsuru"
    sudo mkdir -p /etc/tsuru
    sudo curl -sL https://raw.github.com/globocom/tsuru/master/etc/tsuru-docker.conf -o /etc/tsuru/tsuru.conf
    ssh-keygen -t rsa -f /home/ubuntu/.ssh/id_rsa.pub -N ""
}

function install_docker() {
    sudo apt-get install lxc wget bsdtar curl -y --force-yes
    # are you on AWS? if so, uncomment the line below
    # sudo apt-get install linux-image-extra-`uname -r`
    wget http://get.docker.io/builds/Linux/x86_64/docker-latest.tgz
    tar -xf docker-master.tgz
    cd docker-master
    sudo cp docker /usr/local/bin
    # runs docker daemon, it must be running in order to tsuru work
    sudo docker -d &
}

function start_tsuru() {
    echo "starting beanstalkd"
    sudo service beanstalkd start
    echo "starting tsuru-collector"
    tsr collector &
    echo "starting tsuru-api"
    tsr api &
}

function remove_git_hooks() {
    # this hooks checks if the application is available before receiving a push
    # since docker has nothing before a push, these hooks are not needed
    sudo rm -rf /home/git/bare-template/hooks/pre-receive
    sudo rm -rf /home/git/bare-template/hooks/pre-receive.py
}

function main() {
    source tsuru-setup.bash
    source gandalf-setup.bash
    source hipache-router.bash
    install_docker
    configure_tsuru
    remove_git_hooks
    start_tsuru
}

main
