.. Copyright 2015 tsuru authors. All rights reserved.
   Use of this source code is governed by a BSD-style
   license that can be found in the LICENSE file.

+++++++++++++++++++
Using Pools
+++++++++++++++++++

Overview
========

Pool is used by provisioners to group nodes and know if an application can be
deployed in these nodes. Users can choose which pool to deploy in `tsuru
app-create`.

Tsuru has three types of pool: team, public and default.

Team's pool are segregated by teams, and cloud administrator should set
teams in this pool manually. This pool are just accessible by team's
members.

Public pools are accessible by any user.

Default pool is where apps are deployed when app's team owner don't have a pool
associated with it or when app's creator don't choose any public pool. Ideally
this pool is for experimentation and low profile apps, like service dashboard
and "in development" apps. You can just have one default pool. This is the old
fallback pool, but with a explicit flag.

Adding a pool
-------------

In order to create a pool, you should invoke `tsuru-admin pool-add`:

.. highlight:: bash

::

    $ tsuru-admin pool-add pool1

If you want to create a public pool you can do:

.. highlight:: bash

::

    $ tsuru-admin pool-add pool1 -p

If you want a default pool, you can create it with:

.. highlight:: bash

::

    $ tsuru-admin pool-add pool1 -d

You can overwrite default pool by setting the flag `-f`:

.. highlight:: bash

::

    $ tsuru-admin pool-add new-default-pool -d -f

Adding teams to a pool
----------------------

Then you can use `tsuru-admin pool-teams-add` to add teams to the pool that
you've just created:

.. highlight:: bash

::

    $ tsuru-admin pool-teams-add pool1 team1 team2

    $ tsuru-admin pool-teams-add pool2 team3

Listing pools
-------------

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

If you want to remove a pool, use `tsuru-admin pool-remove`:

.. highlight:: bash

::

    $ tsuru-admin pool-remove pool1


Removing teams from a pool
--------------------------

You can remove one or more teams from a pool using the command `tsuru-admin
pool-teams-remove`:

.. highlight:: bash

::

    $ tsuru-admin pool-teams-remove pool1 team1

    $ tsuru-admin pool-teams-remove pool1 team1 team2 team3
