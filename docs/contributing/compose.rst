.. Copyright 2017 tsuru authors. All rights reserved.
   Use of this source code is governed by a BSD-style
   license that can be found in the LICENSE file.

+++++++++++++++++++++++++++++++++++++++++++++++
Building a development environment with Docker Compose.
+++++++++++++++++++++++++++++++++++++++++++++++

To follow this how-to you need to have Docker_ and Compose_ installed in your machine.

First clone the tsuru_ project from GitHub:

::

    $ git clone https://github.com/tsuru/tsuru.git

Enter the ``tsuru`` directory and execute ``build-compose.sh``. It will
take some time:

::

    $ cd tsuru
    $ sh build-compose.sh

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


And create roles for the admin user:

::

    $ tsuru role-add team-create global
    $ tsuru role-permission-add team-create role.update team.create
    $ tsuru role-add team-member team
    $ tsuru role-permission-add team-member app service-instance team
    $ tsuru role-default-add --team-create team-member
    $ tsuru role-default-add --user-create team-create


You need to create one pool of nodes and add node1 as a tsuru node.
::

    $ tsuru pool-add development
    $ tsuru node-add --register address=http://172.42.0.20:2375 pool=development

Everytime you change tsuru and want to test you need to run ``build-compose.sh`` to build tsurud, generate and run the new api.

.. _Docker: https://docs.docker.com/engine/installation/
.. _Compose: https://docs.docker.com/compose/install/
.. _tsuru: https://github.com/tsuru/tsuru