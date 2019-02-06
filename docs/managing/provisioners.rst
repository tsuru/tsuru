.. Copyright 2017 tsuru authors. All rights reserved.
   Use of this source code is governed by a BSD-style
   license that can be found in the LICENSE file.

++++++++++++
Provisioners
++++++++++++

Provisioners on tsuru are responsible for creating and scheduling units for
applications and node-containers. Originally tsuru supported only one
provisioner called ``docker``. This begin changing with tsuru release 1.2 as
support for `docker swarm mode <https://docs.docker.com/engine/swarm/>`_ and
`Kubernetes <https://kubernetes.io/>`_ as provisioners was added.

Provisioners are also responsible for knowing which nodes are available for the
creation of units, registering new nodes and removing old nodes.

Provisioners are associated to pools and tsuru will use pools to find out which
provisioner is responsible for each application. A single tsuru installation
can manage different pools with different provisioners at the same time.

docker provisioner
------------------

This is the default and original provisioner for tsuru. It comes from a time
where no other scheduler/orchestrator was available for Docker. Neither swarm
nor kubernetes existed yet, so we had to create our own scheduler which uses
the `docker-cluster <https://github.com/tsuru/docker-cluster>`_ library and is
built-in the ``docker`` provisioner.

The provisioner uses MongoDB to store metadata on existing nodes and containers
on each node, and also to track images as they are created on each node. To
accomplish this tsuru talks directly to the Docker API on each node, which must
be allowed to receive connections from the tsuru API using HTTP or HTTPS.

Tsuru relies on the default ``big-sibling`` node-container to monitor
containers on each node and report back containers that are unavailable or that
had its address changed by docker restarting it. The ``docker`` provisioner will
then be responsible for rescheduling such containers on new nodes.

There's no need to register a :doc:`cluster </managing/clusters>` to use the
``docker`` provisioner, simply :doc:`adding new nodes
</installing/adding-nodes>` with Docker API running on them is enough for tsuru
to use them.

Scheduling of units on nodes prioritizes high availability of application
containers. To accomplish this tsuru tries to create each new container on the
node with fewest containers from such application. If there are multiple nodes
with no containers from the application being scheduled tsuru will try to
create new containers on nodes that have different metadata from the ones
containers already exist.

swarm provisioner
-----------------

The ``swarm`` provisioner uses `docker swarm mode
<https://docs.docker.com/engine/swarm/>`_ available in Docker 1.12.0 onward.
Swarm itself is responsible for maintaining available nodes and containers and
tsuru itself doesn't store anything in its internal storage.

To use the ``swarm`` provisioner it's first necessary to register a Swarm
:doc:`cluster </managing/clusters>` in tsuru which must point to a Docker API
server that will behave as a Swarm manager, tsuru itself will do the ``docker
swarm init`` API call if the cluster address is not a Swarm member yet.

Because not all operations are still available through the swarm manager
endpoint (namely commit and push operations) tsuru must still be able to
connect to the docker endpoint of each node directly for such operations. Also,
adding a new node to tsuru will call ``swarm join`` on such node.

Scheduling and availability of containers is completely controlled by the
Swarm, for each tsuru application/process tsuru will create a Swarm ``service``
called ``<application name>-<process name>``. The process of adding and
removing units simply updates the service.

An overlay network is created for each application and every service created
for the application is connected to this same overlay network, allowing
intercommunication directly between containers.

Node containers, e.g big-sibling, are also created as Swarm services with mode
set to ``Global``, which ensures they run every node.

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

Adding new nodes is possible using normal tsuru workflow described in
:doc:`adding new nodes </installing/adding-nodes>`. However, tsuru will only
create a Node resource using the Kubernetes API and will assume that the new
node already has a kubelet process running on it and that it's accessible to
the Kubernetes API server.

Tsuru supports some Kubernetes-specific configurations, check
:doc:`tsuru.yaml docs </using/tsuru.yaml>` for more details.

Kubernetes compatibility
========================

These are the Kubernetes versions that were tested with each tsuru release:

* tsuru <=1.6.2: kubernetes 1.8.x to 1.10.x
* tsuru >=1.7.0: kubernetes 1.10.x to 1.12.x
