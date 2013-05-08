#!/bin/bash

# Copyright 2013 tsuru authors. All rights reserved.
# Use of this source code is governed by a BSD-style
# license that can be found in the LICENSE file.

# This script is used to install tsuru with lxc provisioner.

function install_lxc() {
    echo "Installing lxc"
    sudo apt-get install lxc -y
}

function install_nginx() {
    echo "Installing nginx"
    sudo apt-get install nginx -y
}

function install_extras() {
    echo "Installing git"
    sudo apt-get install git -y
    echo "installing sed"
    sudo apt-get install sed -y
}

function configure_tsuru() {
    echo "Configuring tsuru"
    while [ "$TSURU_DOMAIN" = "" ]
    do
        echo -en "input the primary domain: "; read TSURU_DOMAIN
    done
    [ -d /etc/tsuru ] || sudo mkdir /etc/tsuru
    curl -sL https://raw.github.com/globocom/tsuru/master/etc/tsuru-lxc.conf -o /tmp/tsuru.conf
    sudo bash -c "cat /tmp/tsuru.conf | sed -e \"s/YOURDOMAIN_HERE/$TSURU_DOMAIN/\" > /etc/tsuru/tsuru.conf"
}

function generate_ssh_key() {
    echo "Generating the ssh-key for root"
    [ -f /root/.ssh/id_rsa ] || sudo ssh-keygen -N "" -f /root/.ssh/id_rsa
}

function download_charms() {
    echo "Downloading charms"
    [ -d /home/ubuntu/charms ] || git clone git://github.com/globocom/charms.git -b lxc /home/ubuntu/charms
}

function start_tsuru() {
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
    install_lxc
    install_nginx
    install_extras
    generate_ssh_key
    configure_tsuru
    download_charms
    start_tsuru
}

main
