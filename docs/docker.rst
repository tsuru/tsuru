.. Copyright 2014 tsuru authors. All rights reserved.
   Use of this source code is governed by a BSD-style
   license that can be found in the LICENSE file.

+++++++++++++++++++++++++++++++++++++++++
Build your own PaaS with tsuru and Docker
+++++++++++++++++++++++++++++++++++++++++

This document describes how to create a private PaaS service using tsuru and docker.

This document assumes that tsuru is being installed on a Ubuntu 12.04 LTS 64-bit
machine.

Before install
==============

Before install, let's install curl and python-software-properties, that are used to install extra repositories.

.. highlight:: bash

::

    sudo apt-get update
    sudo apt-get install curl python-software-properties -qqy

Adding repositories
===================

.. highlight:: bash

::

    sudo apt-key adv --keyserver keyserver.ubuntu.com --recv-keys 36A1D7869245C8950F966E92D8576A8BA88D21E9
    echo "deb http://get.docker.io/ubuntu docker main" | sudo tee /etc/apt/sources.list.d/docker.list

    sudo apt-add-repository ppa:tsuru/lvm2 -y
    sudo apt-add-repository ppa:tsuru/ppa -y

    sudo apt-key adv --keyserver hkp://keyserver.ubuntu.com:80 --recv 7F0CEB10
    echo "deb http://downloads-distro.mongodb.org/repo/ubuntu-upstart dist 10gen" | sudo tee /etc/apt/sources.list.d/mongodb.list

    sudo apt-get update

Installing MongoDB
==================

.. highlight:: bash

::

    sudo apt-get install mongodb-10gen -qqy

Installing beanstalkd
=====================

.. highlight:: bash

::

    sudo apt-get install beanstalkd -qqy
    cat > /tmp/default-beanstalkd <<EOF
    BEANSTALKD_LISTEN_ADDR=127.0.0.1
    BEANSTALKD_LISTEN_PORT=11300
    DAEMON_OPTS="-l \$BEANSTALKD_LISTEN_ADDR -p \$BEANSTALKD_LISTEN_PORT -b /var/lib/beanstalkd"
    START=yes
    EOF
    sudo mv /tmp/default-beanstalkd /etc/default/beanstalkd
    sudo service beanstalkd start

Installing redis
================

.. highlight:: bash

::

    sudo apt-get install redis-server -qqy

Installing hipache
==================

.. highlight:: bash

::

    sudo apt-get install node-hipache -qqy
    sudo start hipache

Installing docker
=================

.. highlight:: bash

::

    sudo apt-get install lxc-docker -qqy
    echo export DOCKER_OPTS=\"-H 127.0.0.1:4243\" | sudo tee -a /etc/default/docker
    echo export DOCKER_HOST=127.0.0.1:4243 >> ~/.bashrc
    sudo stop docker
    sudo start docker

Installing gandalf
==================

.. highlight:: bash

::

    sudo apt-get install gandalf-server -qqy
    hook_dir=/home/git/bare-template/hooks
    sudo mkdir -p $hook_dir
    sudo curl https://raw.github.com/globocom/tsuru/master/misc/git-hooks/post-receive -o ${hook_dir}/post-receive
    sudo chmod +x ${hook_dir}/post-receive
    sudo chown -R git:git /home/git/bare-template
    # make sure you write the public IP of the machine in the "host" parameter
    # in the /etc/gandalf.conf file

    sudo start gandalf-server
    sudo start git-daemon

Installing Tsuru API server
===========================

.. highlight:: bash

::

    sudo apt-get install tsuru-server -qqy

    cp /vagrant/tsuru.conf /etc/tsuru/tsuru.conf
    sed -i.old -e "s/{{{HOST_IP}}}/${host_ip}/" /etc/tsuru/tsuru.conf
    sed -i.old -e 's/=no/=yes/' /etc/default/tsuru-server
    rm /etc/default/tsuru-server.old /etc/tsuru/tsuru.conf.old
    start tsuru-ssh-agent
    start tsuru-server-api
    start tsuru-server-collector

Installing platforms
====================

.. highlight:: bash

::

    curl -O https://raw.github.com/globocom/tsuru/master/misc/platforms-setup.js
    mongo tsuru platforms-setup.js

And then install your preferred platform from `basebuilder
<https://github.com/flaviamissi/basebuilder>`_:

.. highlight:: bash

::

    docker build -t tsuru/python https://raw.githubusercontent.com/flaviamissi/basebuilder/master/python/Dockerfile

Replace Python with the desired platform (check basebuilder for a list of
available platforms).

Using tsuru client
==================

Congratulations! At this point you should have a working tsuru server running
on your machine, follow the :doc:`tsuru client usage guide
</apps/client/usage>` to start build your apps.

Adding Services
===============

Here you will find a complete step-by-step example of how to install a mysql
service with tsuru: `http://docs.tsuru.io/en/latest/services/mysql-example.html
<http://docs.tsuru.io/en/latest/services/mysql-example.html>`_

DNS server
==========

You can integrate any DNS server with tsuru. Here:
`<http://docs.tsuru.io/en/latest/misc/dns-forwarders.html>`_ you can find a
example of how to install a DNS server integrated with tsuru
