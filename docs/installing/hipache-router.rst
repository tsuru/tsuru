.. Copyright 2014 tsuru authors. All rights reserved.
   Use of this source code is governed by a BSD-style
   license that can be found in the LICENSE file.

++++++++++++++
Hipache Router
++++++++++++++


`Hipache <https://github.com/hipache/hipache/>`_ is a distributed HTTP and
websocket proxy.

tsuru uses Hipache to route the requests to the containers. Routing information is
stored by tsuru in the configured Redis server, Hipache will read this
configuration directly from Redis.

Adding repositories
===================

Let's start adding the repositories for tsuru which contain the Hipache package.

.. highlight:: bash

::

    sudo apt-get update
    sudo apt-get install python-software-properties
    sudo apt-add-repository ppa:tsuru/ppa -y
    sudo apt-get update

Installing
==========

In order to install Hipache, just use apt-get:

.. highlight:: bash

::

    sudo apt-get install node-hipache


Configuring
===========

In your ``/etc/hipache.conf`` file you must set the ``redisHost`` and
``redisPort`` configuration values. After this, you only need to start hipache
with:

.. highlight:: bash

::

    sudo start hipache
