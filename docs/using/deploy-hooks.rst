.. Copyright 2014 tsuru authors. All rights reserved.
   Use of this source code is governed by a BSD-style
   license that can be found in the LICENSE file.

++++++++++++++++
Deployment hooks
++++++++++++++++

tsuru provides some deployment hooks, like ``restart:before``, ``restart:after``
and ``build``. Deployment hooks allow developers to run commands before and after
some commands.

Hooks are listed in a special file located in the root of the application. The
name of the file may be ``tsuru.yaml`` or ``tsuru.yml``. (``app.yaml`` or
``app.yml`` are also supported, however they're deprecated and ``tsuru.yaml``
should be used.) Here is an example of the file:

.. highlight:: yaml

::

    hooks:
      restart:
        before:
          - python manage.py generate_local_file
        after:
          - python manage.py clear_local_cache
      build:
        - python manage.py collectstatic --noinput
        - python manage.py compress

tsuru supports the following hooks:

* ``restart:before``: this hook lists commands that will run before the unit is
  restarted. Commands listed in this hook will run once per unit. For instance,
  imagine there's an app with two units and the ``tsuru.yaml`` file listed above.
  The command **python manage.py generate_local_file** would run two times, once
  per unit.
* ``restart:after``: this hook is like before-each, but runs after restarting a
  unit.
* ``build``: this hook lists commands that will be run during deploy, when the
  image is being generated.
