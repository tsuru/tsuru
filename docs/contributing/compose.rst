.. Copyright 2017 tsuru authors. All rights reserved.
   Use of this source code is governed by a BSD-style
   license that can be found in the LICENSE file.

++++++++++++++++++++++++++++++++++++++++++++++++++++++
Building a development environment with Docker Compose
++++++++++++++++++++++++++++++++++++++++++++++++++++++

To follow this how-to you need to have Docker_ and Compose_ installed in your machine.

First clone the tsuru_ project from GitHub:

::

    $ git clone https://github.com/tsuru/tsuru.git

Enter the ``tsuru`` directory and execute ``build-compose.sh``. It will
take some time:

::

    $ cd tsuru
    $ ./build-compose.sh

At the first time you run is possible that api and planb fails, just run ``docker-compose up -d`` to fix it.
::

    $ docker-compose up -d

Now you have tsuru dependencies, tsuru api and one docker node running in your machine. You can check
running ``docker-compose ps``:

::

    $ docker-compose ps

You have a fresh tsuru installed, so you need to create the admin user running tsurud inside container. 
For this you need first to kill the running api.

::

    $ docker-compose stop api
    $ docker-compose run --entrypoint="/bin/sh -c" api "tsurud root-user-create admin@example.com"
    $ docker-compose up -d

Then configure the tsuru target:

::

    $ tsuru target-add development http://127.0.0.1:8080 -s

You need to create one pool of nodes and add node1 as a tsuru node.
::

    $ tsuru pool-add development -p -d
    $ tsuru node-add --register address=http://node1:2375 pool=development

Everytime you change tsuru and want to test you need to run ``build-compose.sh`` to build tsurud, generate and run the new api.

If you want to use gandalf, generate one app token and insert into docker-compose.yml file in gandalf environment TSURU_TOKEN.

::

    $ docker-compose stop api
    $ docker-compose run --entrypoint="/bin/sh -c" api "tsurud token"
    // insert token into docker-compose.yml
    $ docker-compose up -d

.. _Docker: https://docs.docker.com/engine/installation/
.. _Compose: https://docs.docker.com/compose/install/
.. _tsuru: https://github.com/tsuru/tsuru
