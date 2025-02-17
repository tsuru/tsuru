.. Copyright 2014 tsuru authors. All rights reserved.
   Use of this source code is governed by a BSD-style
   license that can be found in the LICENSE file.


++++++++++
tsuru.yaml
++++++++++

tsuru.yaml is a special file located in the root of the application. The name of
the file may be ``tsuru.yaml`` or ``tsuru.yml``.

This file is used to describe certain aspects of your app. Currently, it describes
information about deployment hooks and deployment time health checks. The use of 
these features is described bellow.


.. _yaml_deployment_hooks:

Deployment hooks
================

tsuru provides some deployment hooks, like ``restart:before``, ``restart:after``
and ``build``. Deployment hooks allow developers to run commands before and after
some commands.

An example on how to declare these hooks in tsuru.yaml file is described bellow:

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
* ``build``: this hook lists commands that will be run during deployment when the
  image is being generated.

.. _yaml_processes:

Process Configurations
======================

You can declare each process of your app in the tsuru.yaml file.
This is useful because you can use that to configure the command that will be used to run,
the processes and also the healthcheck configurations for each process.

.. highlight:: yaml

::

    processes:
      - name: web
        command: python app.py
        healthcheck:
          path: /
          scheme: http
      - name: web-secondary
        command: python app2.py
        healthcheck:
          path: /
          scheme: http

* ``processes:name``: The name of the process. This field is mandatory.
* ``processes:command``: The command that will be used to run the process. This field is mandatory.
* ``processes:healthcheck``: The healthcheck configuration for the process. This field is optional, and will be described in more detail below.

Healthcheck
===========

You can declare a health check in your tsuru.yaml file. This health check will be
called during the deployment process and tsuru will make sure this health check is
passing before continuing with the deployment process.

If tsuru fails to run the health check successfully it will abort the deployment
before switching the router to point to the new units, so your application will
never be unresponsive. You can configure the maximum time to wait for the
application to respond with the ``docker:healthcheck:max-time`` config.

Health checks may also be used by kubernetes, so
you must ensure that the check is consistent to prevent units from being
temporarily removed from the router.

Example on how you can configure a HTTP based health check in your yaml file:

.. highlight:: yaml

::

    healthcheck:
      path: /healthcheck
      scheme: http
      headers:
        Host: test.com
        X-Custom-Header: xxx
      allowed_failures: 0
      interval_seconds: 10
      timeout_seconds: 60
      deploy_timeout_seconds: 180


Example of a command based healthcheck:

.. highlight:: yaml

::

    healthcheck:
      command: ["curl", "-f", "-XPOST", "http://localhost:8888"]

* ``healthcheck:path``: Which path to call in your application. This path will
  be called for each unit. It is the only mandatory field, if it's not set your
  health check will be ignored. ``Kubernetes expects a status code greater than or
  equal to 200 and less than 400``.
* ``healthcheck:scheme``: Which scheme to use. Defaults to http.
* ``healthcheck:headers``: Additional headers to use for the request. Headers name
  should be capitalized. It is optional.
* ``healthcheck:allowed_failures``: The number of allowed failures before that
  the health check consider the application as unhealthy. Defaults to 3.
* ``healthcheck:timeout_seconds``: The timeout for each healthcheck call in
  seconds. Defaults to 60 seconds.
* ``healthcheck:deploy_timeout_seconds``: The timeout for the first successful
  healthcheck response after the application process has started during a new
  deploy. During this time a new healthcheck attempt will be made every
  ``healthcheck:interval_seconds``. If the healthcheck is not successful in
  this time the deploy will be aborted and rolled back. Defaults to
  :ref:`max-time global config <config_healthcheck_max_time>`.
* ``healthcheck:command``: A command to execute inside the unit container. Exit status 
  of zero is considered healthy and non-zero is unhealthy. This option defaults to an
  empty string array. If ``healthcheck:path`` is set, this option will be ignored.
* ``healthcheck:interval_seconds``: The interval in seconds between each active healthcheck
  call if
* ``healthcheck:force_restart``: Whether the unit should be restarted after ``allowed_failures``
  consecutive healthcheck failures. (Sets the liveness probe in the Pod.)

.. _yaml_kubernetes:

Kubernetes specific configs
===========================

You can configure which ports will be exposed on each process of your app.
Here's a complete example:

.. highlight:: yaml

::

    kubernetes:
      groups:
        pod1:
          process1:
            ports:
              - name: main-port
                protocol: tcp
                target_port: 4123
                port: 8080
              - name: other-port
                protocol: udp
                port: 5000
        pod2:
          process2:

Inside ``groups`` key you can list each pod name - currently tsuru only supports
one process per pod -, and inside each one, the processes names.

For each process, you can configure each exposed port, in ``ports`` key:

* ``kubernetes:groups:<group>:<process>:ports:name``: A descriptive name for the
  port. This field is optional.
* ``kubernetes:groups:<group>:<process>:ports:protocol``: The port protocol.
  The accepted values are ``TCP`` (default) and ``UDP``.
* ``kubernetes:groups:<group>:<process>:ports:target_port``: The port that the
  process is listening on. If omitted, ``port`` value will be used.
* ``kubernetes:groups:<group>:<process>:ports:port``: The port that will be
  exposed on a Kubernetes service. If omitted, ``target_port`` value will be
  used.

If both ``port`` and ``target_port`` are omitted in a port config, the deploy
will fail.

You can set a process to expose no ports (like a worker, for example) with an
empty field, like ``process2`` above.

The configuration for multiple ports still has a couple of limitations:

- healthcheck will be set to use the first configured port in each process
- only the first port of the web process (or the only process, in case there's
  only one) will be exposed in the router - but you can access the other ports
  from other apps in the same cluster, using
  `Kubernetes DNS records <https://kubernetes.io/docs/concepts/services-networking/dns-pod-service/#services>`_,
  like ``appname-processname.namespace.svc.cluster.local``
