.. Copyright 2013 tsuru authors. All rights reserved.
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

    apt-get update
    apt-get install curl -qqy

    apt-get install python-software-properties -qqy

Adding repositories
===================

.. highlight:: bash

::

    curl https://get.docker.io/gpg | apt-key add -
    echo "deb http://get.docker.io/ubuntu docker main" | sudo tee /etc/apt/sources.list.d/docker.list

    apt-add-repository ppa:tsuru/lvm2 -y
    apt-add-repository ppa:tsuru/ppa -y

    apt-key adv --keyserver hkp://keyserver.ubuntu.com:80 --recv 7F0CEB10
    echo "deb http://downloads-distro.mongodb.org/repo/ubuntu-upstart dist 10gen" | sudo tee /etc/apt/sources.list.d/mongodb.list

    apt-get update

Installing mongo
================

.. highlight:: bash

::

    apt-get install mongodb-10gen -qqy

Installing beanstalk
====================

.. highlight:: bash

::
    apt-get install beanstalkd -qqy
    cat > /etc/default/beanstalkd <<EOF
    BEANSTALKD_LISTEN_ADDR=127.0.0.1
    BEANSTALKD_LISTEN_PORT=11300
    DAEMON_OPTS="-l \$BEANSTALKD_LISTEN_ADDR -p \$BEANSTALKD_LISTEN_PORT -b /var/lib/beanstalkd"
    START=yes
    EOF
    service beanstalkd start

Installing redis
================

.. highlight:: bash

::
    apt-get install redis-server -qqy

Installing hipache
==================

.. highlight:: bash

::
    apt-get install node-hipache -qqy
    start hipache

Installing docker
=================

.. highlight:: bash

::
    apt-get install lxc-docker -qqy
    sed -i.old -e 's;-d;-d -H tcp://127.0.0.1:4243;' /etc/init/docker.conf
    rm /etc/init/docker.conf.old
    stop docker
    start docker

Installing gandalf
==================

.. highlight:: bash

::
    apt-get install gandalf-server -qqy
    hook_dir=/home/git/bare-template/hooks
    mkdir -p $hook_dir
    curl https://raw.github.com/globocom/tsuru/master/misc/git-hooks/post-receive -o ${hook_dir}/post-receive
    chmod +x ${hook_dir}/post-receive
    chown -R git:git /home/git/bare-template
    cp /vagrant/gandalf.conf /etc/gandalf.conf
    sed -i.old -e "s/{{{HOST_IP}}}/${host_ip}/" /etc/gandalf.conf

    start gandalf-server
    start git-daemon

Installing Tsuru api server
===========================

.. highlight:: bash

::
    apt-get install tsuru-server -qqy

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
    #git clone https://github.com/flaviamissi/basebuilder
    #(cd basebuilder/python/ && docker -H 127.0.0.1:4243 build -t "tsuru/python" .)

Using tsuru
===========

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
