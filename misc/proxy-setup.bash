#!/bin/bash

# Copyright 2013 tsuru authors. All rights reserved.
# Use of this source code is governed by a BSD-style
# license that can be found in the LICENSE file.

function ask() {
    read -p "Enter http proxy [leave blank for none]: " http_proxy
    if [ "$http_proxy" == "" ]; then
        return
    fi
    read -p "Enter https proxy [${http_proxy}]: " https_proxy
    if [ "$https_proxy" == "" ]; then
        https_proxy=${http_proxy}
    fi
    write_proxy_confs $http_proxy $https_proxy
}

function set_apt() {
    read -d '' apt_template <<"EOF"
Acquire::http::Proxy "s1";
Acquire::https::Proxy "s2";
EOF
    apt_template="${apt_template/s1/$1}"
    apt_template="${apt_template/s2/$2}"
    echo "${apt_template}" | sudo tee -a /etc/apt/apt.conf.d/08proxy #check this cmd
}

function set_profile() {
    read -d '' profile_template <<"EOF"
export http_proxy=s1
export https_proxy=s2
EOF
    profile_template="${profile_template/s1/$1}"
    profile_template="${profile_template/s2/$2}"
    echo "${profile_template}" | sudo tee -a /etc/profile
    source /etc/profile
}

function write_proxy_confs() {
    set_apt $1 $2
    set_profile $1 $2
    echo "Proxy configurations have been saved."
}

ask
