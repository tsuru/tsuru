.. Copyright 2014 tsuru authors. All rights reserved.
   Use of this source code is governed by a BSD-style
   license that can be found in the LICENSE file.

++++++++++
Schedulers
++++++++++

tsuru uses schedulers to chooses which node an unit should be deployed. There are
two schedulers: `round robin` and `segregate scheduler`.

Segregate scheduler
===================

Segregate scheduler is a scheduler that segregates the units between nodes by team.

First, what you need to do is to define a relation between a pool, teams and nodes.
And then, the scheduler deploys the app unit on the pool where a node is related to its team.

    - Pool1
      -> team1, team2
      -> node1

    - Pool2
      -> team2
      -> node3, node4

    - Pool3 (fallback)
      -> <no teams>
      -> node2


Configuration and setup
-----------------------

To use the `segregate scheduler` you shoud enable the segregate mode in
`tsuru.conf` and make sure that the details about the scheduler storage (redis)
is also configured:

.. highlight:: yaml

::

    docker:
      segregate: true
      scheduler:
        redis-server: 127.0.0.1:6379
        redis-prefix: docker-cluster

Adding a pool
-------------

Using `tsuru-admin` you create a pool:

.. highlight:: bash

::

    $ tsuru-admin docker-pool-add pool1

Removing a pool
---------------

A pool is removable if it don't have any node associated with it.
To remove a pool you do:

.. highlight:: bash

::

    $ tsuru-admin docker-pool-remove pool1

Listing a pool
--------------

To list pools you do:

.. highlight:: bash

::

    $ tsuru-admin docker-pool-list
    +-------+-------------------+-----------+
    | Pools | Nodes             | Teams     |
    +-------+-------------------+-----------+
    | pool1 | node1, node2      | team1     |
    | pool2 | node3             | team2     |
    +-------+-------------------+-----------+


Adding node to a pool
---------------------

You can use the `tsuru-admin` to add nodes:

.. highlight:: bash

::

    $ tsuru-admin docker-node-add pool1 http://localhost:4243

Removing a node
---------------

You can use the `tsuru-admin` to remove nodes:

.. highlight:: bash

::

    $ tsuru-admin docker-node-remove pool1 http://localhost:4243
    Node successfully removed.

List nodes
----------

.. highlight:: bash

::

    $ tsuru-admin docker-nodes-list
    +-----------+
    | Address   |
    +-----------+
    | node1     |
    | node2     |
    +-----------+

Adding teams to a pool
-----------------------

You can add one or more teams at once.

.. highlight:: bash

::

    $ tsuru-admin docker-pool-teams-add pool1 team1

    $ tsuru-admin docker-pool-teams-add pool1 team1 team2 team3

Removing teams from a pool
--------------------------

You can remove one or more teams at once.

.. highlight:: bash

::

    $ tsuru-admin docker-pool-teams-remove pool1 team1

    $ tsuru-admin docker-pool-teams-remove pool1 team1 team2 team3

