++++++++++++
api workflow
++++++++++++

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

Tsuru calls your service to create a new instance of your service via POST on ``/resources``(please notice that tsuru does not include a trailing slash) with the "name" that represents the app name in request body. Example of request:

.. highlight:: text

::

    POST /resources HTTP/1.0
    Content-Length: 19

    name=mysql_instance

Your API should return the following HTTP response code with the respective response body:

    * 201: when the instance is successfully created. You don’t need to include any content in the response body.
    * 500: in case of any failure in the creation process. Make sure you include an explanation for the failure in the response body.

Binding an app to a service instance
====================================

This process begins when a Tsuru customer bind an app to an instance of your service via command line tool:

.. highlight:: bash

::

    $ tsuru service bind mysql_instance my_app

Tsuru calls your service to bind an app with a service instance via POST on ``/resources/<service-name>`` (please notice that tsuru does not include a trailing slash) with the "hostname" that represents the app hostname on body. Example of request:

.. highlight:: text

::

    POST /resources/mysql_instance HTTP/1.0
    Content-Length: 25

    hostname=myapp.myhost.com

Your API should return the following HTTP response code with the respective response body:

    * 201: if the app is successfully binded to the instance. The response body must be a JSON containing the environment variables from this instance that should be exported in the app in order to connect to the instance. If your service does not export any environment variable, write ``null`` or ``{}`` in the response body. Example of response:

.. highlight:: text

::

    HTTP/1.1 201 CREATED
    Content-Type: application/json; charset=UTF-8

    {"MYSQL_HOST":"10.10.10.10","MYSQL_PORT":3306,"MYSQL_USER":"ROOT","MYSQL_PASSWORD":"s3cr3t","MYSQL_DATABASE_NAME":"myapp"}

Status codes for errors in the process:

    * 404: if the service instance does not exist. You don't need to include any content in the response body.
    * 412: if the service instance is still being provisioned, and not ready for binding yet. You can optionally include an explanation in the response body.
    * 500: in case of any failure in the bind process. Make sure you include an explanation for the failure in the response body.

Unbind an app from a service instance
=====================================

This process begins when a Tsuru customer unbind an app with an instance of your service via command line tool:

.. highlight:: bash

::

    $ tsuru service unbind mysql_instance my_app

Tsuru calls your service to unbind an app with a service instance via DELETE on ``/resources/<service-name>/hostname/<app-hostname>`` (please notice that tsuru does not include a trailing slash). Example of request:

.. highlight:: text

::

    DELETE /resources/mysql_instance/hostname/myapp.myhost.com HTTP/1.0
    Content-Length: 0

Your API should return the following HTTP response code with respective response body:

    * 200: if the app is successfully unbinded from the instance. You don't need to include any content in the response body.
    * 404: if the service instance does not exist. You don't need to include any content in the response body.
    * 500: in case of any failure in the unbind process. Make sure you include an explanation for the failure in the response body.

Destroying an instance
======================

This process begins when a Tsuru customer remove an instance of your service via command line tool:

.. highlight:: bash

::

    $ tsuru service remove mysql_instance

Tsuru calls your service to remove an isntance of your service via DELETE on ``/resources/<service-name>`` (please notice that tsuru does not include a trailing slash). Example of request:

.. highlight:: text

::

    DELETE /resources/mysql_instance HTTP/1.0
    Content-Length: 0

Your API should return the following HTTP response code with the respective response body:

    * 200: if the service is successfully destroyed. You don’t need to include any content in the response body.
    * 404: if the service instance does not exist. You don’t need to include any content in the response body.
    * 500: in case of any failure in the destroy process. Make sure you include an explanation for the failure in the response body.
