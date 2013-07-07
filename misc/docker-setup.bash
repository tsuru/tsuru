#!/bin/bash -e

# Copyright 2013 tsuru authors. All rights reserved.
# Use of this source code is governed by a BSD-style
# license that can be found in the LICENSE file.

# This script is used to install tsuru with docker provisioner.

function configure_tsuru() {
    echo "Configuring tsuru"
    sudo mkdir -p /etc/tsuru
    sudo -E curl -sL https://raw.github.com/globocom/tsuru/master/etc/tsuru-docker.conf -o /etc/tsuru/tsuru.conf
    # make sure the ubuntu user exists
    if id ubuntu 2>/dev/null >/dev/null; then
        # exists
        true
    else
        sudo useradd -m ubuntu
        sudo -u ubuntu mkdir -p /home/ubuntu/.ssh
    fi
    sudo -u ubuntu ssh-keygen -t rsa -f /home/ubuntu/.ssh/id_rsa.pub -N ""
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
    sudo -E docker -d &
}

function start_tsuru() {
    echo "starting tsuru-collector"
    tsr collector &
    echo "starting tsuru-api"
    tsr api &
}

function configure_git_hooks() {

    # this hooks checks if the application is available before receiving a push
    # since docker has nothing before a push, these hooks are not needed
    sudo rm -rf /home/git/bare-template/hooks/pre-receive
    sudo rm -rf /home/git/bare-template/hooks/pre-receive.py

    # the post-receive hook requires some environment variables to be set
    token=$(/usr/bin/tsr token)
    echo -e "export TSURU_TOKEN=$token\nexport TSURU_HOST=http://127.0.0.1:8080" |sudo -u git tee -a ~git/.bash_profile
}

function use_https_in_git() {
    # this enables npm to work properly
    # npm installs packages using the git readonly url,
    # which won't work behind a proxy
    sudo git config --system url.https://.insteadOf git://
}

function download_scripts() {
    url=https://raw.github.com/globocom/tsuru/master/misc

    for setup in \
        gandalf-setup.bash \
        hipache-setup.bash \
        proxy-setup.bash \
        tsuru-setup.bash \
        ; do
        if [ ! -f $setup  ]; then
            curl -O ${url}/$setup
            chmod +x $setup
        fi
    done
}

function echo_conf_warning() {
    echo "=================================================================================================================="
    echo "        Tsuru is now ready to be started, but first you need to manually set the following configurations"
    echo "        On /etc/tsuru/tsuru.conf set the git:rw-host and git:ro-host to gandalf's public address"
    echo "=================================================================================================================="
}

function main() {
    download_scripts
    source proxy-setup.bash
    source tsuru-setup.bash
    source gandalf-setup.bash
    source hipache-setup.bash
    install_docker
    configure_tsuru
    remove_git_hooks
    configure_git_hooks
    use_https_in_git
    #start_tsuru
    echo_conf_warning
}

main
