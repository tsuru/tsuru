++++++++++++++++++
Build your service
++++++++++++++++++

Overview
========

This docucument is a hands-on guide to turning your existing cloud service into a Tsuru service.

The way to create a service is you implement a provisioning API for your service, which Tsuru will call when a customer create a new instance or bind a service instance with an app.

You will also create a YAML document that will serve as the service manifest. We provide you with a command-line tool to help you to create this manifest and manage your service.



Creating your service api
=========================

To create your api you can use any programming language or framework. In this tutorial we will use `flask <http://flask.pocoo.org>`_.

Prerequisites
-------------

First, let's be shure that Python and pip is already installed:

.. highlight:: bash

::

    $ python --version
    Python 2.7.2

    $ pip
    Usage: pip COMMAND [OPTIONS]

    pip: error: You must give a command (use "pip help" to see a list of commands)

For more information to know how install python you can see the `Python download documentation <http://python.org/download/>`_ and for install pip you can see the `pip installation instructions <http://www.pip-installer.org/en/latest/installing.html>`_.

Now, with python and pip installed, you can use pip to install flask:

.. highlight:: bash

::

    $ pip install flask

With flask installed let's create an file called api.py and added the code to create a minimal flask app:

.. highlight:: python

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

If you open your web browser and access the url "http://127.0.0.1:5000/" you will see the "Hello World!".

In our api it's need implements the resources expecteds by the :doc: `Tsuru api workflow`_.

Provisioning the resource for new instances
-------------------------------------------

Tsuru expects that 

.. highlight:: python

::

    @app.route("/resources", methods=["POST"])
    def add_instance():
        return "", 201

Implementing the bing
---------------------

.. highlight:: python

::

    @app.route("/resources/:name", methods=["POST"])
    def bind(name):
        out = jsonify(SOMEVAR="somevalue")
        return out, 201

Implementing the unbinding
--------------------------

.. highlight:: python

::

    @app.route("/resources/:name", methods=["DELETE"])
    def unbind(name, host):
        return "", 200

Implementing the destroy service instance
-----------------------------------------

.. highlight:: python

::

    @app.route("/resources/:name/host/:host", methods=["DELETE"])
    def remove_instance(name):
        return "", 200

Submiting your service
======================

To submit your service, you can run:

.. highlight:: bash

::

    $ crane create manifest.yaml
