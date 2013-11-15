#!/bin/bash -e

# Copyright 2013 tsuru authors. All rights reserved.
# Use of this source code is governed by a BSD-style
# license that can be found in the LICENSE file.

# This script is used to install tsuru with docker provisioner.

function extra_kenel() {
	echo Installing kernel extra package
	sudo apt-get update
	sudo apt-get install linux-image-extra-`uname -r` -qqy
}

function add_packages() {
	echo Adding Docker repository
	curl https://get.docker.io/gpg | sudo apt-key add -
	sudo /bin/bash -c "echo deb http://get.docker.io/ubuntu docker main > /etc/apt/sources.list.d/docker.list"

	echo Adding Tsuru repository
	apt-add-repository ppa:tsuru/ppa -y

	echo Adding MongoDB repository
	apt-key adv --keyserver hkp://keyserver.ubuntu.com:80 --recv 7F0CEB10
	sudo /bin/bash -c "echo deb http://downloads-distro.mongodb.org/repo/ubuntu-upstart dist 10gen > /etc/apt/sources.list.d/mongodb.list"
}

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
    sudo -u ubuntu ssh-keygen -t rsa -f /home/ubuntu/.ssh/id_rsa -N ""
}


function install_docker() {
    sudo apt-get install bsdtar curl -y --force-yes
    # are you on AWS? if not, comment the line below or get an extra pkg
    sudo apt-get install linux-image-extra-`uname -r` -y --force-yes
    # adding docker repository
    curl https://get.docker.io/gpg | apt-key add -
    sudo /bin/bash -c "echo deb http://get.docker.io/ubuntu docker main > /etc/apt/sources.list.d/docker.list"
    sudo apt-get install lxc-docker -y --force-yes
    # runs docker daemon, it must be running in order to tsuru work
    # Configuring and starting Docker
    sed -i.old -e 's;/usr/bin/docker -d;/usr/bin/docker -H tcp://127.0.0.1:4243 -d;' /etc/init/docker.conf
    rm /etc/init/docker.conf.old
    stop docker
    start docker
}

function start_tsuru() {
    echo "starting tsuru-collector"
    sudo -u ubuntu tsr collector &
    echo "starting tsuru-api"
    sudo -u ubuntu tsr api &
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
    use_https_in_git
    source hipache-setup.bash
    install_docker
    configure_tsuru
    configure_git_hooks
    #start_tsuru
    setup_platforms
    echo_conf_warning
}

main
