#!/bin/bash

# Copyright 2013 tsuru authors. All rights reserved.
# Use of this source code is governed by a BSD-style
# license that can be found in the LICENSE file.

# This script is used to install tsuru with lxc provisioner.

function update_ubuntu() {
    echo "Updating and upgrading"
    sudo apt-get update
    sudo apt-get upgrade -y
}

function install_mongodb() {
    echo "Installing mongodb"
    sudo apt-key adv --keyserver keyserver.ubuntu.com --recv 7F0CEB10
    sudo bash -c 'echo "deb http://downloads-distro.mongodb.org/repo/ubuntu-upstart dist 10gen" > /etc/apt/sources.list.d/10gen.list'
    sudo apt-get update
    sudo apt-get install mongodb-10gen -y
}

function install_beanstalk() {
    echo "Installing beanstalkd"
    sudo apt-get install -y beanstalkd
    sudo sed -i s/#START=yes/START=yes/ /etc/default/beanstalkd
}

function install_tsuru() {
    install_mongodb
    echo "Downloading tsuru binary and copying to /usr/bin"
    curl -sL https://s3.amazonaws.com/tsuru/dist-server/tsr.tar.gz | sudo tar -xz -C /usr/bin
}

function configure_tsuru() {
    echo "Configuring tsuru"
    [ -d /etc/tsuru ] || sudo mkdir /etc/tsuru
    curl -sL https://raw.github.com/globocom/tsuru/master/etc/tsuru-docker.conf -o /etc/tsuru/tsuru.conf
}

function install_gandalf() {
    echo "Installing git"
    sudo apt-get install git -y
    echo "Installing gandalf-wrapper"
    curl -sL https://s3.amazonaws.com/tsuru/dist-server/gandalf-bin.tar.gz | sudo tar -xz -C /usr/bin
    echo "Installing gandalf-webserver"
    curl -sL https://s3.amazonaws.com/tsuru/dist-server/gandalf-webserver.tar.gz | sudo tar -xz -C /usr/bin
}

function configure_gandalf() {
    echo "Configuring gandalf"
    sudo bash -c "echo \"bin-path: /usr/bin/gandalf-bin
    database:
      url: 127.0.0.1:27017
      name: gandalf
    git:
      bare:
        location: /var/repositories
        template: /home/git/bare-template
      daemon:
        export-all: true
    host: $TSURU_DOMAIN
    webserver:
      port: \":8000\"\" > /etc/gandalf.conf"
    echo "Creating git user"
    sudo useradd git
    echo "Creating bare path"
    [ -d /var/repositories ] || sudo mkdir -p /var/repositories
    sudo chown -R git:git /var/repositories
    echo "Creating template path"
    [ -d /home/git/bare-template/hooks ] || sudo mkdir -p /home/git/bare-template/hooks
    sudo curl https://raw.github.com/globocom/tsuru/master/misc/git-hooks/post-receive > /home/git/bare-template/hooks/post-receive
    sudo curl https://raw.github.com/globocom/tsuru/master/misc/git-hooks/pre-receive > /home/git/bare-template/hooks/pre-receive
    sudo curl https://raw.github.com/globocom/tsuru/master/misc/git-hooks/pre-receive.py > /home/git/bare-template/hooks/pre-receive.py
    sudo chmod +x /home/git/bare-template/hooks/*
    sudo chown -R git:git /home/git/bare-template
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

function start_services() {
    echo "starting mongodb"
    sudo service mongodb start
    echo "starting beanstalkd"
    sudo service beanstalkd start
    echo "starting gandalf webserver"
    sudo su - git -c gandalf-webserver &
    echo "starting git daemon"
    sudo su - git -c "git daemon --base-path=/var/repositories --syslog --export-all "&
    echo "starting tsuru-collector"
    tsr collector &
    echo "starting tsuru-api"
    sudo tsr api &
}

function main() {
    update_ubuntu
    install_mongodb
    install_beanstalk
    install_docker
    install_gandalf
    install_tsuru
    configure_gandalf
    configure_tsuru
    start_services
}
