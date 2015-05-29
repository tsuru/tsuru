.. Copyright 2015 tsuru authors. All rights reserved.
   Use of this source code is governed by a BSD-style
   license that can be found in the LICENSE file.

+++++++++++++++++++
Segregate Scheduler
+++++++++++++++++++

Overview
========

tsuru uses schedulers to chooses which node an unit should be deployed.
Previously there was a choice between `round robin` and `segregate scheduler`.
As of 0.11.1, only `segregate scheduler` is available and it's the default
choice. This change was made because `round robin` scheduler was broken,
unmaintained and was a worse scheduling mechanism than `segregate scheduler`.

How it works
============

Segregate scheduler is a scheduler that segregates the units among pools.

First, what you need to do is to define a relation between a pool and teams.
After that you need to register nodes with the ``pool`` metadata information,
indicating to which pool the node belongs.

When deploying an application, the scheduler will choose among the nodes within
the application pool.

Registering a node with pool metadata
-------------------------------------

You can use the ``tsuru-admin`` with ``docker-node-add`` to register or create
nodes with the pool metadata:

.. highlight:: bash

::

    $ tsuru-admin docker-node-add --register address=http://localhost:2375 pool=pool1
