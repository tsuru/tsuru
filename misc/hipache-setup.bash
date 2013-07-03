#!/bin/bash -e

# Copyright 2013 tsuru authors. All rights reserved.
# Use of this source code is governed by a BSD-style
# license that can be found in the LICENSE file.

# This script is used to install the hipache router.

function set_npm_proxy() {
    if [ ! -z "$http_proxy" ]; then
        npm config set proxy $http_proxy
    fi
    if [ ! -z "$https_proxy" ]; then
        npm config set https-proxy $https_proxy
    fi
}

function install_npm() {
    curl http://nodejs.org/dist/v0.8.23/node-v0.8.23-linux-x64.tar.gz | sudo tar -C /usr/local/ --strip-components=1 -zxv
}

function install_redis() {
    sudo apt-get install redis-server -y --force-yes
}

function install_hipache() {
    sudo -E npm install hipache -g
}

function configure_hipache() {
# enable later
#\"https\": {
#    \"port\": 443,
#    \"key\": \"/etc/ssl/ssl.key\",
#    \"cert\": \"/etc/ssl/ssl.crt\"
#}
    sudo bash -c 'echo "
{
    \"server\": {
        \"accessLog\": \"/var/log/hipache_access.log\",
        \"port\": 80,
        \"workers\": 5,
        \"maxSockets\": 100,
        \"deadBackendTTL\": 30
    },
    \"redisHost\": \"127.0.0.1\",
    \"redisPort\": 6379
}" > /etc/hipache.conf.json'
}

function start_hipache() {
    sudo hipache --config /etc/hipache.conf.json &
}

function main() {
    set_npm_proxy
    install_npm
    install_redis
    install_hipache
    configure_hipache
    start_hipache
}

main
