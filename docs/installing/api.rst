.. Copyright 2014 tsuru authors. All rights reserved.
   Use of this source code is governed by a BSD-style
   license that can be found in the LICENSE file.

++++++++++
API Server
++++++++++

Dependencies
============

tsuru API depends on a MongoDB server, Redis server and Hipache router.
Instructions for installing `MongoDB <http://docs.mongodb.org/>`_ and `Redis <http://redis.io/>`_
are outside the scope of this documentation, but it's pretty straight-forward following their
docs. :doc:`installing Hipache </installing/hipache-router>` is described in other session.


Adding repositories
===================

Let's start adding the repositories for tsuru.

For debian based distributions (eg. Ubuntu, Debian)

.. highlight:: bash

::

    $ curl -s https://packagecloud.io/install/repositories/tsuru/stable/script.deb.sh | sudo bash

For rpm based distributions (eg. RedHat, Fedora)

.. highlight:: bash

::

    $ curl -s https://packagecloud.io/install/repositories/tsuru/stable/script.rpm.sh | sudo bash


Installing
==========

.. highlight:: bash

::

    sudo apt-get install tsuru-server -qqy

Now you need to customize the configuration in the ``/etc/tsuru/tsuru.conf``. A
description of possible configuration values can be found in the
:doc:`configuration reference </reference/config>`. A basic possible
configuration is described below, please note that you should replace the values
``your-mongodb-server``, ``your-redis-server`` and ``your-hipache-server``.

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
    queue:
        mongo-url: <your-mongodb-server>:27017
        mongo-database: queuedb
    provisioner: docker
    docker:
        router: hipache
        collection: docker_containers
        repository-namespace: tsuru
        deploy-cmd: /var/lib/tsuru/deploy
        bs:
            image: tsuru/bs:v1
            socket: /var/run/docker.sock
        cluster:
            storage: mongodb
            mongo-url: <your-mongodb-server>:27017
            mongo-database: cluster
        run-cmd:
            bin: /var/lib/tsuru/start
            port: "8888"
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
    or
    $ sudo yum install tsuru-client

    $ tsuru target-add default http://<your-tsuru-api-addr>:8080
    $ tsuru target-set default
    $ tsuru login myemail@somewhere.com
    # type the chosen password


And that's it, you now have registered a user in your tsuru API server and its
ready to run any commands.
