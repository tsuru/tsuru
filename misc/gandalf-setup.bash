#!/bin/bash -e

# Copyright 2013 tsuru authors. All rights reserved.
# Use of this source code is governed by a BSD-style
# license that can be found in the LICENSE file.

# This script is responsible to install and configure gandalf.
# In opposition to the tsuru install script, this one is self contained
# and can be ran standalone in order to install, configure and start gandalf.

function update_ubuntu() {
    echo "Updating and upgrading"
    sudo apt-get update
    sudo apt-get upgrade -y
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
    sudo useradd -m git
    echo "Creating bare path"
    [ -d /var/repositories ] || sudo mkdir -p /var/repositories
    sudo chown -R git:git /var/repositories
    echo "Creating template path"
    [ -d /home/git/bare-template/hooks ] || sudo mkdir -p /home/git/bare-template/hooks
    sudo -E curl https://raw.github.com/globocom/tsuru/master/misc/git-hooks/post-receive -o /home/git/bare-template/hooks/post-receive
    sudo -E curl https://raw.github.com/globocom/tsuru/master/misc/git-hooks/pre-receive -o /home/git/bare-template/hooks/pre-receive
    sudo -E curl https://raw.github.com/globocom/tsuru/master/misc/git-hooks/pre-receive.py -o /home/git/bare-template/hooks/pre-receive.py
    sudo chmod +x /home/git/bare-template/hooks/*
    sudo chown -R git:git /home/git/bare-template
}

function start_gandalf() {
    echo "starting gandalf webserver"
    sudo -u git -i gandalf-webserver &
    echo "starting git daemon"
    sudo -u git -i git daemon --base-path=/var/repositories --syslog --export-all &
}

function main() {
    update_ubuntu
    install_gandalf
    configure_gandalf
    start_gandalf
}

main
