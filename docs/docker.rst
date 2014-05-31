.. Copyright 2014 tsuru authors. All rights reserved.
   Use of this source code is governed by a BSD-style
   license that can be found in the LICENSE file.

++++++++++++++++++++++++
Install tsuru and Docker
++++++++++++++++++++++++

This document describes how to install all tsuru compoments in one virtual machine.
Install all components in one machine is not recommended for production ready but is a good
start to have a tsuru stack working.

tsuru components are composed by:

* MongoDB
* beanstalkd
* hipache
* docker
* gandalf
* tsuru api

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

Let's start adding the docker, mongo and tsuru repositories.

.. highlight:: bash

::

    sudo apt-key adv --keyserver hkp://keyserver.ubuntu.com:80 --recv-keys 36A1D7869245C8950F966E92D8576A8BA88D21E9
    echo "deb http://get.docker.io/ubuntu docker main" | sudo tee /etc/apt/sources.list.d/docker.list

    sudo apt-key adv --keyserver hkp://keyserver.ubuntu.com:80 --recv 7F0CEB10
    echo "deb http://downloads-distro.mongodb.org/repo/ubuntu-upstart dist 10gen" | sudo tee /etc/apt/sources.list.d/mongodb.list

    sudo apt-add-repository ppa:tsuru/ppa -y

    sudo apt-get update


Installing MongoDB
==================

tsuru uses MongoDB to store all data about apps, users and teams. Let's install it:

.. highlight:: bash

::


    sudo apt-get install mongodb-10gen -qqy

Installing beanstalkd
=====================

tsuru uses bealstalk as a queue tool.

.. highlight:: bash

::

    sudo apt-get install beanstalkd -qqy


Change the beastalkd config to enable it. To do it chane the `START=no` to `START=yes`. The configuration
file in in `/etc/default/beanstalkd`.

Now let's start beanstalkd:

.. highlight:: bash

::

    sudo service beanstalkd start

Installing Hipache
==================

Hipache is a distributed HTTP and websocket proxy. tsuru uses Hipache to route
the requests to the containers.

Hipache uses redis to store the router data. To install redis:

.. highlight:: bash

::

    sudo apt-get install redis-server -qqy

We can use apt-get to install Hipache too.

.. highlight:: bash

::

    sudo apt-get install node-hipache -qqy

Now let's start Hipache

.. highlight:: bash

::

    sudo start hipache

Installing docker
=================

.. highlight:: bash

::

    sudo apt-get install lxc-docker -qqy

tsuru uses the docker HTTP api to manage the containers, to it works it is needed to
configure docker to use tcp protocol.

To change it, edit the `/etc/default/docker` adding this line:

.. highlight:: bash

::

    export DOCKER_OPTS=\"-H 127.0.0.1:4243\"

Then restart docker:

.. highlight:: bash

::

    sudo stop docker
    sudo start docker

Installing gandalf
==================

tsuru uses gandalf to manage git repositories.

.. highlight:: bash

::

    sudo apt-get install gandalf-server -qqy

A deploy is executed after a commit happens. To it works, its needed to add a script to be executed
by git post-receive hook.

.. highlight:: bash

::

    hook_dir=/home/git/bare-template/hooks
    sudo mkdir -p $hook_dir
    sudo curl https://raw.githubusercontent.com/tsuru/tsuru/master/misc/git-hooks/post-receive -o ${hook_dir}/post-receive
    sudo chmod +x ${hook_dir}/post-receive
    sudo chown -R git:git /home/git/bare-template
    # make sure you write the public IP of the machine in the "host" parameter
    # in the /etc/gandalf.conf file

    sudo start gandalf-server
    sudo start git-daemon

Installing tsuru API server
===========================

.. highlight:: bash

::

    sudo apt-get install tsuru-server -qqy

    sudo curl http://script.cloud.tsuru.io/conf/tsuru-docker-single.conf -o /etc/tsuru/tsuru.conf
    # make sure you replace all occurrences of {{{HOST_IP}}} with the machine's
    # public IP in the /etc/tsuru/tsuru.conf file
    sudo sed -i -e 's/=no/=yes/' /etc/default/tsuru-server
    sudo start tsuru-ssh-agent
    sudo start tsuru-server-api
    sudo start tsuru-server-collector

Installing platforms
====================

You can use the `tsuru-admin` to install your preferred platform:

.. highlight:: bash

::

    tsuru-admin platform-add platform-name --dockerfile dockerfile-url

For example, Python:

.. highlight:: bash

::

    tsuru-admin platform-add python --dockerfile https://raw.githubusercontent.com/tsuru/basebuilder/master/python/Dockerfile


You can see the oficial tsuru dockerfiles here: https://github.com/tsuru/basebuilder. 

:doc:`Here you can see more docs about tsuru-admin </apps/tsuru-admin/usage>`. 

Using tsuru client
==================

Congratulations! At this point you should have a working tsuru server running
on your machine, follow the :doc:`tsuru client usage guide
</apps/client/usage>` to start build your apps.

Adding Services
===============

Here you will find a complete step-by-step example of how to install a mysql
service with tsuru: :doc:`Install and configure a MySQL service
</services/mysql-example>`.

DNS server
==========

You can integrate any DNS server with tsuru. :doc:`Here you can find an example
of using bind as a DNS forwarder </misc/dns-forwarders>`, integrated with
tsuru.
