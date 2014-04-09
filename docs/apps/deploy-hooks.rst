.. Copyright 2014 tsuru authors. All rights reserved.
   Use of this source code is governed by a BSD-style
   license that can be found in the LICENSE file.

++++++++++++++++
Deployment hooks
++++++++++++++++

tsuru provides some deployment hooks, like ``restart:before``,
``restart:after`` and ``build``. Deployment hooks allow developers to run commands before and
after some commands.

Hooks are listed in a special file located in the root of the application. The
name of the file may be ``app.yaml`` or ``app.yml``. Here is an example of the file:

.. highlight:: yaml

::

    hooks:
      restart:
        before:
          - python manage.py migrate
        before-each:
          - python manage.py generate_local_file
        after-each:
          - python manage.py clear_local_cache
        after:
          - python manage.py clear_redis_cache
      build:
        - python manage.py collectstatic --noinput
        - python manage.py compress

tsuru supports the following hooks:

* ``restart:before``: this hook lists commands that will run before the app is
  restarted. Commands listed in this hook will run once per app.
* ``restart:before-each``: this hook lists commands that will run before the unit is
  restarted. Commands listed in this hook will run once per unit. For instance,
  imagine there's an app with two units and the ``app.yaml`` file listed above.
  The command **python manage.py generate_local_file** would run two times,
  once per unit.
* ``restart:after-each``: this hook is like before-each, but runs after restarting a
  unit.
* ``restart:after``: this hook is like before, but runs after restarting an app.
* ``build``: this hook lists commands that will be run during deploy, when the image is
  being generated. (only for docker provisioner)
