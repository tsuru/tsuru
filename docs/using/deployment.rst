Application Deployment
======================

This document provides a high-level description on how application deployment works on tsuru.

Preparing Your Application
--------------------------

If you follow the `12 Factor <http://www.12factor.net/>`_ app principles you shouldn't have to change
your application in order to deploy it on tsuru. Here is what an application need to go on a tsuru cloud:

 1. Well defined requirements, both, on language level and operational system level
 2. Configuration of external resources using environment variables
 3. A Procfile to tell how your process should be run

Let's go a little deeper through each of those topics.

1. Requirements
+++++++++++++++

Every well writen application nowdays has well defined dependencies. In Python, everything is on a requirements.txt
or like file, in Ruby, they go on Gemfile, Node.js has the package.json, and so on. Some of those dependencies also
have operational system level dependencies, like the Nokogiri Ruby gem or MySQL-Python package, tsuru bootstraps
units as clean as possible, so you also have to declare those operational system requirements you need on a file called
requirements.apt. This files should have the packages declared one per-line and look like that:

::

    python-dev
    libmysqlclient-dev

2. Configuration With Environment Variables
+++++++++++++++++++++++++++++++++++++++++++

Everything that vary between deploys (on different environments, like development or production) should be managed
by environment variables. tsuru takes this principle very seriously, so all services available for usage in tsuru
that requires some sort of configuration does it via environment variables so you have no pain while deploying on
different environments using tsuru.

For instance, if you are going to use a database service on tsuru, like MySQL, when you bind your application into
the service, tsuru will receive from the service API everything you need to connect with MySQL, e.g: user name,
password, url and database name. Having this information, tsuru will export on every unit your application has the
equivalent environment variables with their values. The names of those variables are defined by the service providing
them, in this case, the MySQL service.

Let's take a look at the settings of tsuru hosted application built with Django:

.. highlight:: python

::

    import os

    DATABASES = {
        "default": {
            "ENGINE": "django.db.backends.mysql",
            "NAME": os.environ.get("MYSQLAPI_DB_NAME"),
            "USER": os.environ.get("MYSQLAPI_DB_USER"),
            "PASSWORD": os.environ.get("MYSQLAPI_DB_PASSWORD"),
            "HOST": os.environ.get("MYSQLAPI_HOST"),
            "PORT": "",
            "TEST_NAME": "test",
        }
    }

You might be asking yourself "How am I going to know those variables names?", but don't fear! When you bind your application
with tsuru, it'll return all variables the service asked tsuru to export on your application's units (without the values, since
you are not gonna need them), if you lost the environments on your terminal history, again, don't fear! You can always check
which service made what variables available to your application using the <insert command here>.
