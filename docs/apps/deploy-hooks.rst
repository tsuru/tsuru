.. Copyright 2013 tsuru authors. All rights reserved.
   Use of this source code is governed by a BSD-style
   license that can be found in the LICENSE file.

++++++++++++++++
Deployment hooks
++++++++++++++++

Tsuru provides some deployment hooks, like ``restart:before`` and
``restart:after``. Deployment hooks allow developers to run commands before and
after some commands.

Hooks are listed in a special file located in the root of the application. The
name of the file may be ``app.yaml`` or ``app.yml``. Here is an example of the file:

.. highlight:: yaml

::

    hooks:
      restart:
        before:
          - python manage.py collectstatic --noinput
          - python manage.py compress
        before-each:
          - python manage.py generate_local_file
        after-each:
          - python manage.py clear_local_cache
        after:
          - python manage.py clear_redis_cache

Currently, Tsuru supports the following restart hooks:

* ``before``: this hook lists commands that will run before the app is
  restarted. Commands listed in this hook will run once per app.
* ``before-each``: this hook lists commands that will run before the unit is
  restarted. Commands listed in this hook will run once per unit. For instance,
  imagine there's an app with two units and the ``app.yaml`` file listed above.
  The command **python manage.py generate_local_file** would run two times,
  once per unit.
* ``after-each``: this hook is like before-each, but runs after restarting a
  unit.
* ``after``: this hook is like before, but runs after restarting an app.
