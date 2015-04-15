.. Copyright 2015 tsuru authors. All rights reserved.
   Use of this source code is governed by a BSD-style
   license that can be found in the LICENSE file.

+++++++++++++++++++
Using Pools
+++++++++++++++++++

Overview
========

Pool is used by provisioners to group nodes and know if an app can be deployed in these nodes.
Users can choose what pool to deploy in `tsuru app-create`.

Configuration and setup
-----------------------

Adding a pool
-------------

Using `tsuru-admin` you create a pool:

.. highlight:: bash

::

    $ tsuru-admin pool-add pool1

Adding teams to a pool
-----------------------

You can add one or more teams at once.

.. highlight:: bash

::

    $ tsuru-admin pool-teams-add pool1 team1 team2

    $ tsuru-admin pool-teams-add pool2 team3

Listing a pool
--------------

To list pools you do:

.. highlight:: bash

::

    $ tsuru-admin pool-list
    +-------+-------------+
    | Pools | Teams       |
    +-------+-------------+
    | pool1 | team1 team2 |
    | pool2 | team3       |
    +-------+-------------+

Removing a pool
---------------

To remove a pool you do:

.. highlight:: bash

::

    $ tsuru-admin pool-remove pool1


Removing teams from a pool
--------------------------

You can remove one or more teams at once.

.. highlight:: bash

::

    $ tsuru-admin pool-teams-remove pool1 team1

    $ tsuru-admin pool-teams-remove pool1 team1 team2 team3
