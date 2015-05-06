.. Copyright 2015 tsuru authors. All rights reserved.
   Use of this source code is governed by a BSD-style
   license that can be found in the LICENSE file.

Choose a pool to deploy your app
================================

tsuru has a concept of pool, a group of machines that will run the application
code. Pools are defined by the cloud admin as needed and users can choose one of
them in the moment of app creation.

Users can see which pools are available using the command `tsuru pool-list`:

.. highlight:: bash

::

    $ tsuru pool-list

    +---------+--------------+
    | Team    | Pools        |
    +---------+--------------+
    | team1   | pool1, pool2 |
    +---------+--------------+

So, in `app-create`, users can choose the pool using the `-o/--pool pool_name`
flag:

.. highlight:: bash

::

    $ tsuru app-create app_name platform -o pool1

There's no need to specify the pool when the user has access to only one pool.
