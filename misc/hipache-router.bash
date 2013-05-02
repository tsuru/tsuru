#!/bin/bash -e

# Copyright 2013 tsuru authors. All rights reserved.
# Use of this source code is governed by a BSD-style
# license that can be found in the LICENSE file.

# This script is used to install the hipache router.

function install_npm() {
    sudo apt-get install npm -y --force-yes
}

function install_redis() {
    sudo apt-get install redis-server -y --force-yes
}

function install_hipache() {
    sudo npm install hipache -g
}

function configure_hipache() {
    sudo bash -c "echo \"
{
    \"server\": {
        \"accessLog\": \"/var/log/hipache_access.log\",
        \"port\": 80,
        \"workers\": 5,
        \"maxSockets\": 100,
        \"deadBackendTTL\": 30,
        \"https\": {
            \"port\": 443,
            \"key\": \"/etc/ssl/ssl.key\",
            \"cert\": \"/etc/ssl/ssl.crt\"
        }
    },
    \"redisHost\": \"127.0.0.1\",
    \"redisPort\": 6379
}\" > /etc/hipache.conf.json"
}

function start_hipache() {
    sudo hipache --config /etc/hipache.conf.json
}

function main() {
    install_npm
    install_redis
    install_hipache
    configure_hipache
}

main
