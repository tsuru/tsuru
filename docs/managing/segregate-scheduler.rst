.. Copyright 2015 tsuru authors. All rights reserved.
   Use of this source code is governed by a BSD-style
   license that can be found in the LICENSE file.

+++++++++++++++++++
Segregate Scheduler
+++++++++++++++++++

Overview
========

tsuru uses schedulers to chooses which node an unit should be deployed. There are
two schedulers: `round robin` and `segregate scheduler`.

The default one is `round robin`, this page describes what the `segregate
scheduler` does and how to enable it.

How it works
============

Segregate scheduler is a scheduler that segregates the units between nodes by
team.

First, what you need to do is to define a relation between a pool and teams. After
that you need to register nodes with the ``pool`` metadata information, indicating
to which pool the node belongs.

When deploying an application, the scheduler will choose among the nodes with the
pool metadata information associated to the team owning the application being
deployed.

Configuration and setup
-----------------------

To use the `segregate scheduler` you need to enable the segregate mode in
``tsuru.conf``:

.. highlight:: yaml

::

    docker:
      segregate: true


Registering a node with pool metadata
-------------------------------------

You can use the ``tsuru-admin`` with ``docker-node-add`` to register or create
nodes with the pool metadata:

.. highlight:: bash

::

    $ tsuru-admin docker-node-add --register address=http://localhost:2375 pool=pool1
