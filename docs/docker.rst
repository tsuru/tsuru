.. Copyright 2014 tsuru authors. All rights reserved.
   Use of this source code is governed by a BSD-style
   license that can be found in the LICENSE file.

++++++++++++++++++++++++
Install tsuru and Docker
++++++++++++++++++++++++

This document describes how to manually install all tsuru compoments in one
virtual machine. Installing all components in one machine is not recommended
for production ready but is a good start to have a tsuru stack working.

You can install it automatically using `tsuru-now
<https://github.com/tsuru/now>`_ (or `tsuru-bootstrap
<https://github.com/tsuru/tsuru-bootstrap>`_, that runs tsuru-now on vagrant).

tsuru components are composed by:

* MongoDB
* Redis
* Hipache
* Docker
* Gandalf
* tsuru API

This document assumes that tsuru is being installed on a Ubuntu Server 14.04
LTS 64-bit machine.

Before install
==============

Before install, let's install curl and python-software-properties, that are
used to install extra repositories.

.. highlight:: bash

::

    sudo apt-get update
    sudo apt-get install curl python-software-properties -qqy

Adding repositories
===================

Let's start adding the repositories for Docker, tsuru and MongoDB.

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


    sudo apt-get install mongodb-org -qqy

Installing Redis
================

tsuru uses Redis for message queueing and Hipache uses it for storing routing
data.

.. highlight:: bash

::

    sudo apt-get install redis-server -qqy

Installing Hipache
==================

Hipache is a distributed HTTP and websocket proxy. tsuru uses Hipache to route
the requests to the containers.

In order to install Hipache, just use apt-get:

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

    export DOCKER_OPTS="-H 127.0.0.1:4243"

Then restart docker:

.. highlight:: bash

::

    sudo stop docker
    sudo start docker

Installing gandalf and archive-server
=====================================

tsuru uses gandalf to manage git repositories.

.. highlight:: bash

::

    sudo apt-get install gandalf-server -qqy

A deploy is executed in the ``git push``. In order to get it working, you will
need to add a pre-receive hook. Tsuru comes with three pre-receive hooks, all
of them need further configuration:

    * s3cmd: uses `Amazon S3 <https://s3.amazonaws.com>`_ to store and server
      archives
    * archive-server: uses tsuru's `archive-server
      <https://github.com/tsuru/archive-server>`_ to store and serve archives
    * swift: uses `Swift <http://swift.openstack.org>`_ to store and server
      archives (compatible with `Rackspace Cloud Files
      <http://www.rackspace.com/cloud/files/>`_)

In this tutorial, we will use archive-server, but you can use anything that can
store a git archive and serve it via HTTP or FTP. You can install
archive-server via apt-get too:

.. highlight:: bash

::

    sudo apt-get install archive-server -qqy

Then you will need to configure Gandalf, install the pre-receive hook, set the
proper environment variables and start Gandalf and the archive-server:

.. highlight:: bash

::

    hook_dir=/home/git/bare-template/hooks
    sudo mkdir -p $hook_dir
    sudo curl https://raw.githubusercontent.com/tsuru/tsuru/master/misc/git-hooks/pre-receive.archive-server -o ${hook_dir}/pre-receive
    sudo chmod +x ${hook_dir}/pre-receive
    sudo chown -R git:git /home/git/bare-template
    cat | sudo tee -a /home/git/.bash_profile <<EOF
    export ARCHIVE_SERVER_READ=http://172.17.42.1:3232 ARCHIVE_SERVER_WRITE=http://127.0.0.1:3131
    EOF

In the ``/etc/gandalf.conf`` file, remove the comment from the line "template:
/home/git/bare-template", so it looks like that:

.. highlight:: yaml

::

    git:
      bare:
        location: /var/lib/gandalf/repositories
        template: /home/git/bare-template

Then start gandalf and archive-server:

.. highlight:: bash

::

    sudo start gandalf-server
    sudo start archive-server

Installing tsuru API server
===========================

.. highlight:: bash

::

    sudo apt-get install tsuru-server -qqy

    sudo sed -i -e 's/=no/=yes/' /etc/default/tsuru-server
    sudo start tsuru-ssh-agent
    sudo start tsuru-server-api
    sudo start tsuru-server-collector

Now you need to customize the configuration in the ``/etc/tsuru/tsuru.conf``.

.. highlight:: bash

::

    sudo vim /etc/tsuru/tsuru.conf


The basic configuration is:

::

    listen: "0.0.0.0:8080"
    debug: true
    host: http://machine-public-ip:8080 # This port must be the same as in the "listen" conf
    admin-team: admin
    auth:
        user-registration: true
        scheme: native # you can use oauth or native
    database:
        url: 127.0.0.1:27017
        name: tsurudb
    queue: redis
    redis-queue:
        host: 127.0.0.1
        port: 6379


Now we will configure git:

::

    git:
        unit-repo: /home/application/current
        api-server: http://127.0.0.1:8000

Finally, we will configure docker:

::

    provisioner: docker
    docker:
        segregate: false
        servers:
            - http://127.0.0.1:4243
        router: hipache
        collection: docker_containers
        repository-namespace: tsuru
        deploy-cmd: /var/lib/tsuru/deploy
        ssh-agent-port: 4545
        scheduler:
            redis-server: 127.0.0.1:6379
            redis-prefix: docker-cluster
        run-cmd:
            bin: /var/lib/tsuru/start
            port: "8888"
        ssh:
            add-key-cmd: /var/lib/tsuru/add-key
            public-key: /var/lib/tsuru/.ssh/id_rsa.pub
            user: ubuntu
    hipache:
        domain: tsuru-sample.com # tsuru uses this to mount the app's urls

All confs are better explained `here <http://tsuru.readthedocs.org/en/latest/config.html>`_.

Generating token for Gandalf authentication
===========================================

The last step before is to tell the pre-receive script where to find the tsuru
server and how to talk to it. We do that by exporting two environment variables
in the ``~git/.bash_profile`` file:

.. highlight:: bash

::

    cat | sudo tee -a /home/git/.bash_profile <<EOF
    export TSURU_HOST=http://127.0.0.1:8080
    export TSURU_TOKEN=`tsr token`
    EOF

Using tsuru client
==================

Congratulations! At this point you should have a working tsuru server running
on your machine, follow the :doc:`tsuru client usage guide
</apps/client/usage>` to start build your apps.

Installing platforms
====================

After creating the first user and the admin team, you can use the `tsuru-admin`
to install your preferred platform:

.. highlight:: bash

::

    tsuru-admin platform-add platform-name --dockerfile dockerfile-url

For example, Python:

.. highlight:: bash

::

    tsuru-admin platform-add python --dockerfile https://raw.githubusercontent.com/tsuru/basebuilder/master/python/Dockerfile


You can see the oficial tsuru dockerfiles here: https://github.com/tsuru/basebuilder.

:doc:`Here you can see more docs about tsuru-admin </apps/tsuru-admin/usage>`.

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
