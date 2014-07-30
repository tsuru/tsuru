.. Copyright 2014 tsuru authors. All rights reserved.
   Use of this source code is governed by a BSD-style
   license that can be found in the LICENSE file.

+++++++++++++++++++++
Building your service
+++++++++++++++++++++

.. _`service manifest`: `Creating a service manifest`_

Overview
========

This document is a hands-on guide to turning your existing cloud service into a
tsuru service.

In order to create a service you need to implement a provisioning API for your
service, which tsuru will call using `HTTP protocol
<http://en.wikipedia.org/wiki/Hypertext_Transfer_Protocol#Request_methods>`_
when a customer creates a new instance or binds a service instance with an app.

You will also need to create a YAML document that will serve as the service
manifest. We provide a command-line tool to help you to create this manifest
and manage your service.

Creating your service api
=========================

To create your service API, you can use any programming language or framework.
In this tutorial we will use `flask <http://flask.pocoo.org>`_.

Authentication
==============

tsuru will authenticate with your service API using HTTP basic authentication.
The user is the name of the service and the password is defined in the `service
manifest`_.

Using Flask, you can manage basic authentication using a decorator described in
this Flask snippet: http://flask.pocoo.org/snippets/8/.

Prerequisites
-------------

First, let's be sure that Python and pip are already installed:

.. highlight:: bash

::

    $ python --version
    Python 2.7.2

    $ pip
    Usage: pip COMMAND [OPTIONS]

    pip: error: You must give a command (use "pip help" to see a list of commands)

For more information about how to install python you can see the `Python
download documentation <http://python.org/download/>`_ and about how to install
pip you can see the `pip installation instructions
<http://www.pip-installer.org/en/latest/installing.html>`_.

Now, with python and pip installed, you can use pip to install flask:

.. highlight:: bash

::

    $ pip install flask

With flask installed let's create a file called api.py and add the code to
create a minimal flask app:

.. highlight:: python

::

    from flask import Flask
    app = Flask(__name__)

    @app.route("/")
    def hello():
        return "Hello World!"

    if __name__ == "__main__":
        app.run()

For run this app you can do:

.. highlight:: bash

::

    $ python api.py
     * Running on http://127.0.0.1:5000/

If you open your web browser and access the url "http://127.0.0.1:5000/" you
will see the "Hello World!".

Then, you need to implement the resources expected by the :doc:`tsuru api
workflow </services/api>`.

Provisioning the resource for new instances
-------------------------------------------

For new instances tsuru sends a POST to /resources with the "name" that
represents the service instance name in the request body. If the service
instance is successfully created, your API should return 201 in status code.

Let's create a method for this action:

.. highlight:: python

::

    @app.route("/resources", methods=["POST"])
    def add_instance():
        return "", 201

Implementing the bind
---------------------

In the bind action, tsuru calls your service via POST on
/resources/<service_name>/ with the "app-hostname" that represents the app
hostname and the "unit-hostname" that represents the unit hostname on body.

If the app is successfully binded to the instance, you should return 201 as
status code with the variables to be exported in the app environment on body
with the json format.

As an example, let's create a method that returns a json with a fake variable
called "SOMEVAR" to be injected in the app environment. To do it in flask you
need to import the jsonify method.

.. highlight:: python

::

    from flask import jsonify

    @app.route("/resources/<name>", methods=["POST"])
    def bind(name):
        out = jsonify(SOMEVAR="somevalue")
        return out, 201

Implementing the unbinding
--------------------------

In the unbind action, tsuru calls your service via DELETE on
/resources/<service_name>/hostname/<unit_hostname>/.

If the app is successfully unbinded from the instance you should return 200 as
status code.

Let's create a method for this action:

.. highlight:: python

::

    @app.route("/resources/<name>/hostname/<host>", methods=["DELETE"])
    def unbind(name, host):
        return "", 200

Implementing the destroy service instance
-----------------------------------------

In the destroy action, tsuru calls your service via DELETE on
/resources/<service_name>/.

If the service instance is successfully removed you should return 200 as status
code.

Let's create a method for this action:

.. highlight:: python

::

    @app.route("/resources/<name>", methods=["DELETE"])
    def remove_instance(name):
        return "", 200

Implementing the url for status checking
----------------------------------------

To check the status of an instance, tsuru uses the url
``/resources/<service_name>/status``. If the instance is ok, this URL should
return 204.

Let's create a function for this action:

.. highlight:: python

::

    @app.route("/resources/<name>/status", methods=["GET"])
    def status(name):
        return "", 204

The final code for our "fake api" developed in flask is:

.. highlight:: python

::

    from flask import Flask
    from flask import jsonify

    app = Flask(__name__)


    @app.route("/resources/<name>", methods=["POST"])
    def bind(name):
        out = jsonify(SOMEVAR="somevalue")
        return out, 201


    @app.route("/resources/<name>/hostname/<host>", methods=["DELETE"])
    def unbind(name, host):
        return "", 200


    @app.route("/resources", methods=["POST"])
    def add_instance():
        return "", 201


    @app.route("/resources/<name>", methods=["DELETE"])
    def remove_instance(name, host):
        return "", 200


    @app.route("/resources/<name>/status", methods=["GET"])
    def status(name):
        return "", 204


    if __name__ == "__main__":
        app.run()


Creating a service manifest
===========================

Using crane you can create a manifest template:

.. highlight:: bash

::

    $ crane template

This will create a manifest.yaml in your current path with this content:

.. highlight:: yaml

::

    id: servicename
    endpoint:
        production: production-endpoint.com
        test: test-endpoint.com:8080

The manifest.yaml is used by crane to defined an id and an endpoint to your
service.

Change the id and the endpoint values with the information of your service:

.. highlight:: yaml

::

    id: fakeserviceid1
    password: secret123
    endpoint:
        production: fakeserviceid1.com

Submiting your service
======================

To submit your service, you can run:

.. highlight:: bash

::

    $ crane create manifest.yaml
