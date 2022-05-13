.. Copyright 2021 tsuru authors. All rights reserved.
   Use of this source code is governed by a BSD-style
   license that can be found in the LICENSE file.

:title: Installing Tsuru in a Google Kubernetes Engine cluster with Helm

.. _installing_tsuru_gke:

Installing Tsuru in a Google Kubernetes Engine cluster with Helm
================================================================

This post will show how to install and configure Tsuru in a GKE Helm.
All steps in this guide were done in Kubernetes v1.22.0. While it might work for almost all Kubernetes versions, some versions may break something. Feel free to report us in that case.
You need to have both gcloud and kubectl previously installed on your machine, if you don't have it yet, you can install it `here <https://cloud.google.com/sdk/docs/install/>`_, with gcloud and `kubectl <https://kubernetes.io/docs/tasks/tools/>`_ properly installed, let's get started.
To create a Kubernetes cluster using gcloud, run the command:

.. highlight:: bash

::

    $ gcloud beta container clusters create tsuru-cluster --image-type=COS --machine-type=e2-standard-4 --num-nodes "2" --zone=$YOUR_PREEFERED_ZONE


Download a release of the `Helm client <https://github.com/helm/helm/releases>`_. With helm installed, let's start

Installing Tsuru
----------------

To install Tsuru and its dependencies we will use a helm chart

.. highlight:: bash

::

    $ helm repo add tsuru https://tsuru.github.io/charts

Now let's install the chart!

.. highlight:: bash

::

    $ helm install tsuru tsuru/tsuru-stack --create-namespace --namespace tsuru-system

Now you have tsuru installed!!

Configuring Tsuru
-----------------

Create the admin user on tsuru:

.. highlight:: bash

::

    $ kubectl exec -it -n tsuru-system deploy/tsuru-api -- tsurud root user create admin@admin.com# CHANGE IT TO YOUR ADMIN USER #


Add the tsuru target and log in:

.. highlight:: bash

::

   $ export TSURU_HOST=$(kubectl get svc -n tsuru-system tsuru-api -o 'jsonpath={.status.loadBalancer.ingress[].ip}')
   $ tsuru target-add gke http://$TSURU_HOST -s
   $ tsuru login

Create one team:

.. highlight:: bash

::

   $ tsuru team create admin

Build Platforms:

.. highlight:: bash

::

   $ tsuru platform add python
   $ tsuru platform add go

Create and Deploy tsuru-dashboard app:

.. highlight:: bash

::

   $ tsuru app create dashboard
   $ tsuru app deploy -a dashboard --image tsuru/dashboard

Create an app to test:

.. highlight:: bash

::

   $ mkdir example-go
   $ cd example-go
   $ git clone https://github.com/tsuru/platforms.git
   $ cd platforms/examples/go
   $ tsuru app create example-go go
   $ tsuru app deploy -a example-go .

Check the app info and get the url:

.. highlight:: bash

::

   $ tsuru app info -a example-go
