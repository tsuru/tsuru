.. Copyright 2017 tsuru authors. All rights reserved.
   Use of this source code is governed by a BSD-style
   license that can be found in the LICENSE file.

++++++++++++
Provisioners
++++++++++++

Provisioners on tsuru are responsible for creating and scheduling units for
applications. Initially tsuru supported only one
provisioner called ``docker``. This begin changing with tsuru release 1.2 as
support for `Kubernetes <https://kubernetes.io/>`_ as provisioners, now it is the default provisioner.

Provisioners are also responsible for knowing which nodes are available for the
creation of units, registering new nodes and removing old nodes.

Provisioners are associated to pools and tsuru will use pools to find out which
provisioner is responsible for each application. A single tsuru installation
can manage different pools with different provisioners at the same time.

Kubernetes provisioner
----------------------

The ``kubernetes`` provisioner uses `Kubernetes <https://kubernetes.io/>`_ to
manage nodes and containers, tsuru also doesn't store anything in its internal
storage related to nodes and containers. It's first necessary to register a
Kubernetes :doc:`cluster </managing/clusters>` in tsuru which must point to the
Kubernetes API server.

Scheduling is controlled exclusively by Kubernetes, for each
application/process tsuru will create a Deployment controller. Changes to the
application like adding and removing units are executed by updating the
Deployment with rolling update configured using the Kubernetes API. Node
containers are created using the DaemonSets.

A Service controller is also created for every Deployment, this allows direct
communication between services without the need to go through a tsuru router.

Tsuru supports some Kubernetes-specific configurations, check
:doc:`tsuru.yaml docs </using/tsuru.yaml>` for more details.

Kubernetes compatibility
========================

These are the Kubernetes versions that were tested with each tsuru release:

* tsuru <=1.6.2: kubernetes 1.8.x to 1.10.x
* tsuru >=1.7.0: kubernetes 1.10.x to 1.12.x
* tsuru >=1.9.0: kubernetes 1.14.x to 1.18.x
