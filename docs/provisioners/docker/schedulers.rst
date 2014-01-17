.. Copyright 2014 tsuru authors. All rights reserved.
   Use of this source code is governed by a BSD-style
   license that can be found in the LICENSE file.

++++++++++
Schedulers
++++++++++

Tsuru uses schedulers to choices which node an unit should be deployed. There is
two schedulers: `round robin` and `segregate scheduler`.

Segregate scheduler
===================

Segregate scheduler is a scheduler that segregates the units between nodes by team.

How it works, you make a relation between nodes and teams. And the scheduler deploys
the app unit on the nodes defineds for you team.

    - team1 -> node1
    - team2 -> node3
    - others -> fallback (node4)

Configuration and setup
-----------------------

To use the `segregate scheduler` you shoud enable the segregate mode in 
`tsuru.conf` and make sure that the conf about scheduler storage (redis)
is configured:

.. highlight:: yaml

::

    docker:
      segregate: true
      scheduler:
        redis-server: 127.0.0.1:6379
        redis-prefix: docker-cluster

Adding a node
-------------

You can use the `tsr` to add nodes:

.. highlight:: bash

::

    $ tsr docker-add-node someid http://localhost:4243 myteam


Adding a fallback node
----------------------

To add a fallback is just to add a node without team:

.. highlight:: bash

::

    $ tsr docker-add-node someid http://localhost:4243

Removing a node
---------------

You can use the `tsr` to remove nodes: 

.. highlight:: bash

::

    $ tsr docker-rm-node xxx
    Node successfully removed.

List nodes
----------

Just use `docker-list-nodes` to list nodes:

.. highlight:: bash

::

    $ tsr docker-list-nodes
    +------+-----------------------+------------------+
    | ID   | Address               | Team             |
    +------+-----------------------+------------------+
    | fall | http://localhost:4243 |                  |
    | xpto | http://localhost:4243 | xpto             |
    +------+-----------------------+------------------+
