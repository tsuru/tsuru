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
app create`.

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

In order to create a pool, you should invoke `tsuru pool add` or `create a terraform resource`:

.. tabs::

   .. tab:: Tsuru client

      .. highlight:: bash

      ::

          $ tsuru pool add pool1

   .. tab:: Terraform

      .. highlight:: terraform

      ::

          resource "tsuru_pool" "pool1" {
            name = "pool1"
          }


If you want to create a public pool you can do:

.. tabs::

   .. tab:: Tsuru client

      .. highlight:: bash

      ::

          $ tsuru pool add pool1 --public


   .. tab:: Terraform

      .. highlight:: terraform

      ::

          resource "tsuru_pool" "pool1" {
            name  = "pool1"
            public = true
          }

If you want a default pool, you can create it with:

.. tabs::

   .. tab:: Tsuru client

      .. highlight:: bash

      ::

          $ tsuru pool add pool1 --default

   .. tab:: Terraform

      .. highlight:: terraform

      ::

          resource "tsuru_pool" "pool1" {
            name    = "pool1"
            default = true
          }


You can overwrite default pool by setting the flag `-f`:

.. highlight:: bash

::

    $ tsuru pool add new-default-pool -d -f

Adding teams to a pool
----------------------

Then you can use `tsuru pool constraint set` to add teams to the pool that
you've just created:

.. highlight:: bash

::

    $ tsuru pool constraint set pool1 team team1 team2 --append

    $ tsuru pool constraint set pool2 team team3 --append

Listing pools
-------------

To list pools you do:

.. highlight:: bash

::

    $ tsuru pool list
    +-------+-------------+
    | Pools | Teams       |
    +-------+-------------+
    | pool1 | team1 team2 |
    | pool2 | team3       |
    +-------+-------------+

Removing a pool
---------------

If you want to remove a pool, use `tsuru pool remove`:

.. highlight:: bash

::

    $ tsuru pool remove pool1


Removing teams from a pool
--------------------------

You can remove one or more teams from a pool using the command `tsuru pool constraint set`:

.. highlight:: bash

::

    $ tsuru pool constraint set pool1 team team1 --blacklist

    $ tsuru pool constraint set pool1 team team1 team2 team3 --blacklist

Removing services from a pool
-----------------------------

You can remove one or more services from a pool using the command `tsuru pool constraint set`:

.. highlight:: bash

::

    $ tsuru pool constraint set <pool> service <service1> <service2> <serviceN> --blacklist

    $ tsuru pool constraint set dev_pool service mongo_prod mysql_prod --blacklist

Moving apps between pools and teams
-----------------------------------

You can move apps from poolA to poolB and from teamA to teamB even when they dont have permission to see each other's pools, this is made by using `tsuru app update`:

.. highlight:: bash

::

    $ tsuru app update --app <app> --team-owner <teamB> --pool <poolB>

By default the app will be set to both teams, so teamA can still see the app just in case that the user may have made some mistake. If you wish to remove the old teamA from the app, It's possible using `tsuru app revoke`:

.. highlight:: bash

::

    $ tsuru app revoke teamA -a <app>
