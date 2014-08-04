.. Copyright 2014 tsuru authors. All rights reserved.
   Use of this source code is governed by a BSD-style
   license that can be found in the LICENSE file.

++++++++++
API Server
++++++++++

Dependencies
============

tsuru API depends on a Mongodb server, Redis server, Hipache router, and Gandalf
server. Instructions for installing `Mongodb <http://docs.mongodb.org/>`_ and
`Redis <http://redis.io/>`_ are outside the scope of this documentation, but it's
pretty straight-forward following their docs. :doc:`Installing Gandalf
</installing/gandalf>` and :doc:`installing Hipache </installing/hipache-router>`
were described in other sessions.


Adding repositories
===================

Let's start adding the repositories for tsuru.

.. highlight:: bash

::

    sudo apt-get update
    sudo apt-get install python-software-properties
    sudo apt-add-repository ppa:tsuru/ppa -y
    sudo apt-get update


Installing
==========

.. highlight:: bash

::

    sudo apt-get install tsuru-server -qqy
    sudo sed -i -e 's/=no/=yes/' /etc/default/tsuru-server
    sudo start tsuru-server-api

Now you need to customize the configuration in the ``/etc/tsuru/tsuru.conf``. A
description of possible configuration values can be found in the
:doc:`configuration reference </reference/config>`. A basic possible
configuration is described below, please note that you should replace the values
``your-mongodb-server``, ``your-redis-server``, ``your-gandalf-server`` and
``your-hipache-server``.

.. highlight:: yaml

::

    listen: "0.0.0.0:8080"
    debug: true
    host: http://<machine-public-addr>:8080 # This port must be the same as in the "listen" conf
    admin-team: admin
    auth:
        user-registration: true
        scheme: native
    database:
        url: <your-mongodb-server>:27017
        name: tsurudb
    queue: redis
    redis-queue:
        host: <your-redis-server>
        port: 6379
    git:
        unit-repo: /home/application/current
        api-server: http://<your-gandalf-server>:8000
    provisioner: docker
    docker:
        segregate: false
        router: hipache
        collection: docker_containers
        repository-namespace: tsuru
        deploy-cmd: /var/lib/tsuru/deploy
        ssh-agent-port: 4545
        cluster:
            storage: mongodb
            mongo-url: <your-mongodb-server>:27017
            mongo-database: cluster
        run-cmd:
            bin: /var/lib/tsuru/start
            port: "8888"
        ssh:
            add-key-cmd: /var/lib/tsuru/add-key
            user: ubuntu
    hipache:
        domain: <your-hipache-server>.xip.io
        redis-server: <your-redis-server-with-port>


.. _gandalf_auth_token:

Generating token for Gandalf authentication
===========================================

Assuming you have already configured your Gandalf server in the :doc:`previous
installation step </installing/gandalf>`, we need to export two extra environment
variables to the git user, which will run our deploy hooks, the URL to our API
server and a generated token.

First step is to generate a token in the machine we've just installed the API
server:

.. highlight:: bash

::

    $ tsr token
    fed1000d6c05019f6550b20dbc3c572996e2c044


Now you have to go back to the machine you installed Gandalf, and run this:

.. highlight:: bash

::

    $ cat | sudo tee -a /home/git/.bash_profile <<EOF
    export TSURU_HOST=http://<your-tsuru-api-addr>:8080
    export TSURU_TOKEN=fed1000d6c05019f6550b20dbc3c572996e2c044
    EOF

