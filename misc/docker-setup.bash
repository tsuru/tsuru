#!/bin/bash

sudo apt-get install lxc wget bsdtar curl
# are you on AWS? if so, uncomment the line below
# sudo apt-get install linux-image-extra-`uname -r`

wget http://get.docker.io/builds/$(uname -s)/$(uname -m)/docker-master.tgz
tar -xf docker-master.tgz
cd docker-master
sudo cp docker /usr/local/bin

# runs docker daemon, it must be running in order to tsuru work
sudo docker -d &
