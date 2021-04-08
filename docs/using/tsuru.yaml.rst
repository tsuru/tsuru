.. Copyright 2014 tsuru authors. All rights reserved.
   Use of this source code is governed by a BSD-style
   license that can be found in the LICENSE file.


++++++++++
tsuru.yaml
++++++++++

tsuru.yaml is a special file located in the root of the application. The name of
the file may be ``tsuru.yaml`` or ``tsuru.yml``.

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

Health checks may also be used by the application router and by kubernetes, so
you must ensure that the check is consistent to prevent units from being
temporarily removed from the router.

Example on how you can configure a HTTP base health check in your yaml file:

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

      // Ignored in kubernetes provisioner pools:
      method: GET
      status: 200
      use_in_router: false
      match: .*OKAY.*
      router_body: content


Example of a command based healthcheck (kubernetes only):

.. highlight:: yaml

::

    healthcheck:
      command: ["curl", "-f", "-XPOST", "http://localhost:8888"]

* ``healthcheck:path``: Which path to call in your application. This path will
  be called for each unit. It is the only mandatory field, if it's not set your
  health check will be ignored.
* ``healthcheck:scheme``: Which scheme to use. Defaults to http.
* ``healthcheck:method``: The method used to make the http request. This field is
  ignored in kubernetes provisioner, GET is always used. Defaults to GET.
* ``healthcheck:status``: The expected response code for the request. Defaults
  to 200. This field is ignored in ``kubernetes`` provisioner, which always
  expects a status code greater than or equal to 200 and less than 400.
* ``healthcheck:headers``: Additional headers to use for the request. Headers name
  should be capitalized. It is optional.
* ``healthcheck:match``: A regular expression to be matched against the request
  body. This field is ignored in kubernetes provisioner, use
  ``healthcheck:command`` if a more complex healthcheck is necessary. If it's
  not set the body won't be read and only the status code will be checked. This
  regular expression uses `Go syntax
  <https://code.google.com/p/re2/wiki/Syntax>`_ and runs with ``.`` matching
  ``\n`` (``s`` flag).
* ``healthcheck:allowed_failures``: The number of allowed failures before that
  the health check consider the application as unhealthy. Defaults to 3 on
  kubernetes pools and 0 on docker pools.
* ``healthcheck:use_in_router``: Whether this health check path should also be
  registered in the router. This field is ignored in ``kubernetes``
  provisioner, which constantly calls the healthcheck every
  ``interval_seconds``. Defaults to false in other provisioners. When an app
  has no explicit healthcheck or use_in_router is false a default healthcheck
  is configured.
* ``healthcheck:router_body``: Body content passed to the router when
  ``use_in_router`` is true. This field is ignored in kubernetes provisioner,
  use ``healthcheck:command`` if a more complex healthcheck is necessary.
* ``healthcheck:timeout_seconds``: The timeout for each healthcheck call in
  seconds. Defaults to 60 seconds.
* ``healthcheck:deploy_timeout_seconds``: The timeout for the first successful
  healthcheck response after the application process has started during a new
  deploy. During this time a new healthcheck attempt will be made every
  ``healthcheck:interval_seconds``. If the healthcheck is not successful in
  this time the deploy will be aborted and rolled back. Defaults to
  :ref:`max-time global config <config_healthcheck_max_time>`.
* ``healthcheck:command``: Exclusive to the ``kubernetes`` provisioner. A
  command to execute inside the unit container. Exit status of zero is
  considered healthy and non-zero is unhealthy. This option defaults to an
  empty string array. If ``healthcheck:path`` is set, this option will be
  ignored.
* ``healthcheck:interval_seconds``: Exclusive to the ``kubernetes``
  provisioner. The interval in seconds between each active healthcheck call if
  ``use_in_router`` is set to true. Defaults to 10 seconds.
* ``healthcheck:force_restart``: Exclusive to the ``kubernetes``
  provisioner. Whether the unit should be restarted after ``allowed_failures``
  consecutive healthcheck failures. (Sets the liveness probe in the Pod.)


.. _yaml_kubernetes:

Kubernetes specific configs
===========================

If your app is running on a Kubernetes provisioned pool, you can set specific
configurations for Kubernetes. These configurations will be ignored if your app
is running on another provisioner.

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
