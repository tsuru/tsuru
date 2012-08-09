++++++++++++++++++
Build your service
++++++++++++++++++

Overview
========

This docucument is a hands-on guide to turning your existing cloud service into a Tsuru service.

The way to create a service is you implement a provisioning API for your service, which Tsuru will call when a customer create a new instance or bind a service instance with an app.

You will also create a YAML document that will serve as the service manifest. We provide you with a command-line tool to help you to create this manifest and manage your service.

Prerequisites
=============


Creating your service api
=========================

Submiting your service
======================

To submit your service, you can run:

.. highlight:: bash

::

    $ crane create manifest.yaml
