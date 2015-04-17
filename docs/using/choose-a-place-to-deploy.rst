Choose a place to deploy your app
=================================

Tsuru has a concept of pool, a group of machines that will run your app.
Pools are defined by your cloud admin as needed and you can choose one of them
in the moment of app create.

To see what pools are available to you, just do `tsuru pool-list`.

.. highlight:: bash

::

    $ tsuru pool-list

    +---------+--------------+
    | Team    | Pools        |
    +---------+--------------+
    | team1   | pool1, pool2 |
    +---------+--------------+

So, in app create you can choose the pool using the `-o/--pool pool_name` flag:

.. highlight:: bash

::

    $ tsuru app-create app_name platform -o pool1

If you have just one pool you don't need to specify it.
