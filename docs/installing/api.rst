.. Copyright 2016 tsuru authors. All rights reserved.
   Use of this source code is governed by a BSD-style
   license that can be found in the LICENSE file.

++++++++++
API Server
++++++++++

Dependencies
============

tsuru API depends on a MongoDB server, Redis server, Hipache router, and
Gandalf server. Instructions for installing `MongoDB
<http://docs.mongodb.org/>`_ and `Redis <http://redis.io/>`_ are outside the
scope of this documentation, but it's pretty straight-forward following their
docs. :doc:`Installing Gandalf </installing/gandalf>` and :doc:`installing
Hipache </installing/hipache-router>` are described in other sessions.


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
    auth:
        user-registration: true
        scheme: native
    database:
        url: <your-mongodb-server>:27017
        name: tsurudb
    pubsub:
        redis-host: <your-redis-server>
        redis-port: 6379
    queue:
        mongo-url: <your-mongodb-server>:27017
        mongo-database: queuedb
    git:
        api-server: http://<your-gandalf-server>:8000
    provisioner: docker
    docker:
        router: hipache
        collection: docker_containers
        repository-namespace: tsuru
        deploy-cmd: /var/lib/tsuru/deploy
        bs:
            image: tsuru/bs:v1
            reporter-interval: 10
            socket: /var/run/docker.sock
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
    routers:
        hipache:
            type: hipache
            domain: <your-hipache-server-ip>.xip.io
            redis-server: <your-redis-server-with-port>


In particular, take note that you must set ``auth:user-registration`` to ``true``:

.. highlight:: yaml

::

    auth:
        user-registration: true
        scheme: native


Otherwise, tsuru will fail to create an admin user in the next section.

Now you only need to start your tsuru API server:


.. highlight:: bash

::

    sudo sed -i -e 's/=no/=yes/' /etc/default/tsuru-server
    sudo start tsuru-server-api


Creating admin user
===================

The creation of an admin user is necessary before interaction with the API is
possible. This can be done using the ``root-user-create`` command as shown
below. This command will create a new authorization role with a global
permission allowing this user run any action on tsuru. More fine-grained roles
can be created later, please refer to :doc:`managing users and permissions
</managing/users-and-permissions>` for more details.

Here we're also going to describe how to install the ``tsuru`` client
application. For a description of each command shown below please refer to the
:doc:`client documentation </reference/tsuru-client>`.

For a description

.. highlight:: bash

::

    $ tsurud root-user-create [--config <path to tsuru.conf>] myemail@somewhere.com
    # type a password and confirmation (only if using native auth scheme)

    $ sudo apt-get install tsuru-client
    $ tsuru target-add default http://<your-tsuru-api-addr>:8080
    $ tsuru target-set default
    $ tsuru login myemail@somewhere.com
    # type the chosen password


And that's it, you now have registered a user in your tsuru API server and its 
ready to run any commands.


.. _gandalf_auth_token:

Generating token for Gandalf authentication
===========================================

Assuming you have already configured your Gandalf server in the :doc:`previous
installation step </installing/gandalf>`, now we need to export two extra
environment variables to the git user, which will run our deploy hooks, the URL
to our API server and a generated token.

First step is to generate a token in the machine we've just installed the API
server:

.. highlight:: bash

::

    $ tsurud token
    fed1000d6c05019f6550b20dbc3c572996e2c044


Now you have to go back to the machine you installed Gandalf, and run this:

.. highlight:: bash

::

    $ cat | sudo tee -a /home/git/.bash_profile <<EOF
    export TSURU_HOST=http://<your-tsuru-api-addr>:8080
    export TSURU_TOKEN=fed1000d6c05019f6550b20dbc3c572996e2c044
    EOF

