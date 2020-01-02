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

In order to follow this guide, you need installing the Docker_ (v1.13.0 or later),
`Docker Compose`_ (v1.10.0 or later) and the `Tsuru client`_ (newest possible).
After getting these tools, make sure they are running correctly on your system.

.. _Docker:  https://docs.docker.com/engine/installation/
.. _`Docker Compose`: https://docs.docker.com/compose/install/
.. _Tsuru: https://github.com/tsuru/tsuru
.. _`Tsuru client`: https://tsuru-client.readthedocs.io/en/latest/installing.html

Running Docker Compose
----------------------

Pull the latest source code available at Tsuru's repository at GitHub. Navigate
to the newly-created ``tsuru`` directory and run the Docker Compose to up.

.. code:: bash

   $ git clone https://github.com/tsuru/tsuru.git
   $ cd tsuru
   $ docker-compose up -d

.. NOTE::

  For making new changes in the Tsuru server effective, you need to rebuild and
  run the ``api`` service. This can be done by running the ``docker-compose up
  --build -d api`` command.

Whether everything works as expected, you have a fresh and ready to use
installation of Tsuru.

Creating admin user
-------------------

To be able to manage that Tsuru installation, you need create a administrator user
who is able to execute any privileged action on Tsuru. You can do that executing the
command shown below.

.. code:: bash

    $ docker-compose exec api tsurud root-user-create admin@tsuru.example.com

That command will prompt a password and its confirmation. Make sure to remeber the
chosen credential, it will be used in the next step.

Login on Tsuru API
------------------

Create a new target pointing to the local Tsuru API, then log on.

.. code:: bash

    $ tsuru target-add -s development http://127.0.0.1:8080
    $ tsuru login admin@tsuru.example.com

The login command will prompt the password for you, fill that with the credential
entered before.

Adding a Docker pool
--------------------

Create a new pool (using the default provisioner - ``docker``),  so add the
Docker node into.

.. code:: bash

  $ tsuru pool-add --public --default docker-pool
  $ tsuru node-add --register address=http://node:2375 pool=docker-pool


Adding a Kubernetes pool
------------------------

You can also integrating with a Kubenertes cluster provided via Minikube.

.. code:: bash

  $ minikube start --insecure-registry=registry.tsuru.172.42.0.21.nip.io:5000

Create a pool in tsuru to be managed by the cluster:

.. code:: bash

  $ tsuru pool-add kube-pool --provisioner kubernetes

Register your minikube as a tsuru cluster:

.. code:: bash

  $ tsuru cluster-add minikube kubernetes \
      --pool kube-pool \
      --addr https://$(minikube ip):8443 \
      --cacert ~/.minikube/ca.crt \
      --clientcert ~/.minikube/apiserver.crt \
      --clientkey ~/.minikube/apiserver.key

.. code:: bash

  $ tsuru node-update $(tsuru node-list -q -f tsuru.io/cluster=kube-pool) pool=kube-pool

.. _Minikube: https://kubernetes.io/docs/setup/learning-environment/minikube/

You are ready to create and deploy apps either to Docker or Kubernetes pools.

Cleaning up
-----------

To erase all Tsuru installation made above, you can just run.
.. code:: bash

  $ docker-compose down --volumes --rmi all
  $ minikube delete

