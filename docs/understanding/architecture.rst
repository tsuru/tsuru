.. Copyright 2014 tsuru authors. All rights reserved.
   Use of this source code is governed by a BSD-style
   license that can be found in the LICENSE file.

============
Architecture
============

API
---

The API component (also referred as the tsuru daemon, or `tsurud`) is a RESTful
API server written with ``Go``. The API is responsible for the deploy workflow
and the lifecycle of applications.

Command-line clients and the `tsuru dashboard <https://github.com/tsuru/tsuru-dashboard>`_ interact with this component.

Database
--------

The database component is a `MongoDB <https://www.mongodb.org/>`_ server.

Kubernetes
-----------

The default provisioner is `Kubernetes <http://kubernetes.io/>`_.

Registry
--------

The `Docker registry <https://github.com/docker/docker-registry>`_ is the component responsible for storing and distributing `Docker <https://www.docker.com/>`_ images.

Router
------

The router component routes traffic from users to applications. The recommended implementation of router is `kubernetes-router <https://github.com/tsuru/kubernetes-router>` that manages kubernetes loadbalancers and ingresses.
