.. Copyright 2017 tsuru authors. All rights reserved.
   Use of this source code is governed by a BSD-style
   license that can be found in the LICENSE file.

++++++++++++++++++++++++++++++++++++++++++++++++++++++
Building a development environment with Docker Compose
++++++++++++++++++++++++++++++++++++++++++++++++++++++

This guide shows you how to run the Tsuru on a single host using Docker Compose.
That is mainly useful for development and test environments since it
allows you quickly to start and down the required components of Tsuru.

.. WARNING::
  No security and high availability concerns were employed in this installation
  method. Do not run it on production environments.

In order to follow this guide, you need installing the Docker_ (v1.13.0 or
later), `Docker Compose`_ (v1.10.0 or later), Minikube_ (v1.6 or later) and the
`Tsuru client`_ (newest possible). After getting these tools, make sure they
are running correctly on your system.

.. _Docker: https://docs.docker.com/engine/installation/
.. _`Docker Compose`: https://docs.docker.com/compose/install/
.. _Minikube: https://kubernetes.io/docs/tasks/tools/install-minikube/
.. _Tsuru: https://github.com/tsuru/tsuru
.. _`Tsuru client`: https://tsuru-client.readthedocs.io/en/latest/installing.html

Running Docker Compose
----------------------

Pull the latest Tsuru's source code from GitHub and navigate to its root
directory.

.. code:: bash

   $ git clone https://github.com/tsuru/tsuru.git
   $ cd tsuru/

Then, run the Docker Compose to start the side components as well as the Tsuru
server.

.. code:: bash

  $ docker-compose up -d

.. NOTE::
  Whenever you change the Tsuru source code, you will need to rebuild and run
  the new Tsuru server to see that change working. Do it by running the
  ``docker-compose up --build -d api`` command.

If everything works as expected, you have a fresh and ready to use
installation of Tsuru.

Creating admin user
-------------------

To be able to manage that Tsuru installation, you will need creating an
administrator user who can perform any privileged action. Run the command below
to create that.

.. code:: bash

    $ docker-compose exec api tsurud root-user-create admin@tsuru.example.com

.. NOTE::
  The email used above is only illustrative. Feel free to use a more convenient.

That command will prompt a password and its confirmation. Make sure to remeber
the chosen credential, it will be used in the next steps.

Logging into Tsuru
------------------

Create a new target pointing to the local Tsuru API, then log on.

.. code:: bash

    $ tsuru target-add -s development http://127.0.0.1:8080
    $ tsuru login admin@tsuru.example.com

This login command will prompt the password for you, fill that with the
credential entered before.

Adding a Docker pool
--------------------

Create a new pool (under the ``docker`` provisioner - the default one), and add
the Docker node within it.

.. code:: bash

  $ tsuru pool-add --public --default docker-pool
  $ tsuru node-add --register address=http://node:2375 pool=docker-pool

Wait until the above node be ready. You can check the node status by running
the below command.

.. code:: bash

  $ tsuru node-info http://node:2375

Adding a Kubernetes pool
------------------------

Tsuru applications can also be orchestrated by the Kubernetes. In order that
you will need a live Kubernetes cluster. In this guide, the Minikube tool is
used to provide quick and minimal installation of.

Create a pool under the Kubernetes provisioner.

.. code:: bash

  $ tsuru pool-add kube-pool --provisioner kubernetes

Create a local Kubernetes cluster using the Minikube tool.

.. code:: bash

  $ minikube start --insecure-registry=$(docker-compose exec api sh -c 'echo ${REGISTRY_URL}')

Wait until the Minikube installation over, so register it on Tsuru clusters.

.. code:: bash

  $ tsuru cluster-add minikube kubernetes \
      --pool kube-pool \
      --addr https://$(minikube ip):8443 \
      --cacert ~/.minikube/ca.crt \
      --clientcert ~/.minikube/apiserver.crt \
      --clientkey ~/.minikube/apiserver.key

Make the Kubernetes master node a regular one.

.. code:: bash

  $ tsuru node-update $(tsuru node-list -q -f tsuru.io/cluster=minikube) pool=kube-pool

You are ready to create and deploy apps either to Docker or Kubernetes pools.

Cleaning up
-----------

To erase all installation made here, you can execute the commands below.

.. code:: bash

  $ docker-compose down --volumes --rmi all
  $ minikube delete

