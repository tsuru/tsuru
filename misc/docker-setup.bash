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
    wget -q http://get.docker.io/builds/Linux/x86_64/docker-latest.tgz
    tar -xf docker-latest.tgz
    cd docker-latest
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

function use_https_in_git() {
    # this enables npm to work properly
    # npm installs packages using the git readonly url,
    # which won't work behind a proxy
    sudo git config --system url.https://.insteadOf git://
}

function download_scripts() {
    if [ ! -f proxy-setup.bash ]; then
        curl -O https://raw.github.com/globocom/tsuru/master/misc/proxy-setup.bash
        chmod +x proxy-setup.bash
    fi
    if [ ! -f tsuru-setup.bash ]; then
        curl -O https://raw.github.com/globocom/tsuru/master/misc/tsuru-setup.bash
        chmod +x tsuru-setup.bash
    fi
    if [ ! -f gandalf-setup.bash ]; then
        curl -O https://raw.github.com/globocom/tsuru/master/misc/gandalf-setup.bash
        chmod +x gandalf-setup.bash
    fi
    if [ ! -f hipache-setup.bash ]; then
        curl -O https://raw.github.com/globocom/tsuru/master/misc/hipache-setup.bash
        chmod +x hipache-setup.bash
    fi
}

function main() {
    download_scripts
    source proxy-setup.bash
    source tsuru-setup.bash
    source gandalf-setup.bash
    source hipache-router.bash
    install_docker
    configure_tsuru
    remove_git_hooks
    use_https_in_git
    start_tsuru
}

main
