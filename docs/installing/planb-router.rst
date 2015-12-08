.. Copyright 2015 tsuru authors. All rights reserved.
   Use of this source code is governed by a BSD-style
   license that can be found in the LICENSE file.

++++++++++++
PlanB Router
++++++++++++

`PlanB <https://github.com/tsuru/planb/>`_ is a distributed HTTP and
websocket proxy. It's built on top of a configuration pattern defined by
`Hipache <https://github.com/hipache/hipache/>`_.

tsuru uses PlanB to route the requests to the containers. Routing information is
stored by tsuru in the configured Redis server, PlanB will read this
configuration directly from Redis.

Adding repositories
===================

Let's start adding the repositories for tsuru which contain the PlanB package.

.. highlight:: bash

::

    sudo apt-get install software-properties-common -y
    sudo apt-add-repository ppa:tsuru/ppa -y
    sudo apt-get update

Installing
==========

In order to install PlanB, just use apt-get:

.. highlight:: bash

::

    sudo apt-get install planb -y

Configuring
===========

You may change the file ``/etc/default/planb``, changing the PLANB_OPTS
environment variable for configuring the binding address and the Redis
endpoint, along with other settings, as `described in PlanB docs
<https://github.com/tsuru/planb#start-up-flags>`_.

After changing the file, you only need to start PlanB with:

.. highlight:: bash

::

    sudo start planb
