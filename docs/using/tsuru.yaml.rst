.. Copyright 2015 tsuru authors. All rights reserved.
   Use of this source code is governed by a BSD-style
   license that can be found in the LICENSE file.


++++++++++
tsuru.yaml
++++++++++

tsuru.yaml is a special file located in the root of the application. The name of
the file may be ``tsuru.yaml`` or ``tsuru.yml``. (``app.yaml`` or ``app.yml`` are
also supported for backward compatibility reasons, however this will be dropped
soon.)

This file is used to describe certain aspects of your app. Currently it describes
information about deployment hooks and deployment time health checks. How to use
this features is described below.


.. _yaml_deployment_hooks:

Deployment hooks
================

tsuru provides some deployment hooks, like ``restart:before``, ``restart:after``
and ``build``. Deployment hooks allow developers to run commands before and after
some commands.

Here is an example about how to declare this hooks in your tsuru.yaml file:

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


.. _yaml_healthcheck:

Healthcheck
===========

You can declare a health check in your tsuru.yaml file. This health check will be
called during the deployment process and tsuru will make sure this health check is
passing before continuing with the deployment process.

If tsuru fails to run the health check successfully it will abort the deployment
before switching the router to point to the new units, so your application will
never be unresponsive. You can configure the maximum time to wait for the
application to respond with the ``docker:healthcheck:max-time`` config.

Here is how you can configure a health check in your yaml file:

.. highlight:: yaml

::

    healthcheck:
      path: /healthcheck
      method: GET
      status: 200
      match: .*OKAY.*
      allowed_failures: 0

* ``healthcheck:path``: Which path to call in your application. This path will be
  called for each unit. It is the only mandatory field, if it's not set your
  health check will be ignored.
* ``healthcheck:method``: The method used to make the http request. Defaults to
  GET.
* ``healthcheck:status``: The expected response code for the request. Defaults to
  200.
* ``healthcheck:match``: A regular expression to be matched against the request
  body. If it's not set the body won't be read and only the status code will be
  checked. This regular expression uses `go syntax
  <https://code.google.com/p/re2/wiki/Syntax>`_ and runs with ``.`` matching
  ``\n`` (``s`` flag).
* ``healthcheck:allowed_failures``: The number of allowed failures before that the 
  health check consider the application as unhealthy. Defaults to 0.
