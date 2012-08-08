+++
api
+++

Tsuru sends a request to your service for:

* create a new instance of your service
* bind an app with your service
* unbind an app
* destroy an instance

Creating a new instance
=======================

This process begins when a Tsuru customer create an instance of your service via command line tool:

.. highlight:: bash

::

    $ tsuru service add mysql mysql_instance

Tsuru calls your service to create a new instance of your service via POST on /resources/ with the "name" that represents the app name in request body.

If the service instance is successfully created, your API should return 201 in status code. If an error happen you should return a 500 in status code.

Binding an app
==============

This process begins when a Tsuru customer bind an app with an instance of your service via command line tool:

.. highlight:: bash

::

    $ tsuru service bind mysql_instance my_app

Tsuru calls your service to bind an app with a service instance via POST on /resources/mysql_instance/ with the "hostname" that represents the app hostname on body.

Your API should return 404 when service instance does not exists. On error, you should return the 500 for status code with the error message on body.

If the app is successfully binded to the instance you should return 201 for status code with the variables to be exported in app environ on body with the json format.

Unbind an app
=============

This process begins when a Tsuru customer unbind an app with an instance of your service via command line tool:

.. highlight:: bash

::

    $ tsuru service unbind mysql_instance my_app

Tsuru calls your service to unbind an app with a service instance via DELETE on /resources/mysql_instance/hostname/127.0.0.1/.
On error, you should return the 500 for status code with the error message on body.

If the app is successfully unbinded from the instance you should use 204 as status code.

Destroying an instance
======================

This process begins when a Tsuru customer remove an instance of your service via command line tool:

.. highlight:: bash

::

    $ tsuru service remove mysql_instance

Tsuru calls your service to remove an isntance of your service via DELETE on /resources/mysql_instance/.
On error, you should return the 500 for status code with the error message on body.

If the service instance is successfully removed you should use 204 as status code.
