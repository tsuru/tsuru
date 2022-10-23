.. Copyright 2021 tsuru authors. All rights reserved.
   Use of this source code is governed by a BSD-style
   license that can be found in the LICENSE file.

:title: Installing Tsuru using the makefile recipe

.. _installing_tsuru_local:

Installing Tsuru using the makefile recipe
========================================================

This post will show how to install Tsuru using the makefile recipe.

Prequisites
---------------------
* minikube and kubectl. if you don't have it yet, you can install it `here <https://minikube.sigs.k8s.io/docs/start/>`_, with minikube and `kubectl <https://kubernetes.io/docs/tasks/tools/>`_ properly installed, you are good to go.


Installing Tsuru
----------------

Install Tsuru and its dependencies: 

.. highlight:: bash

::

    $ sudo make local


Note: In some cases, you may get a 'permission denied', in such cases make sure you have remove all traces of the "minikube" cluster:

.. highlight:: bash

::

    $ minikube delete
