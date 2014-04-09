.. Copyright 2013 tsuru authors. All rights reserved.
   Use of this source code is governed by a BSD-style
   license that can be found in the LICENSE file.

++++++++++++
api workflow
++++++++++++

tsuru sends requests to your service to:

* create a new instance of your service
* bind an app with your service
* unbind an app
* destroy an instance

Creating a new instance
=======================

This process begins when a tsuru customer creates an instance of your service
via command line tool:

.. highlight:: bash

::

    $ tsuru service-add mysql mysql_instance

tsuru calls your service to create a new instance of your service via POST on
``/resources`` (please notice that tsuru does not include a trailing slash)
with the "name" that represents the app name in the request body. Example of
request:

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

This process begins when a tsuru customer binds an app to an instance of your service via command line tool:

.. highlight:: bash

::

    $ tsuru bind mysql_instance --app my_app

tsuru calls your service to bind an app with a service instance via POST on ``/resources/<service-name>`` (please notice that tsuru does not include a trailing slash) with the "hostname" that represents the app hostname in the request body. Example of request:

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

This process begins when a tsuru customer unbinds an app from an instance of your service via command line tool:

.. highlight:: bash

::

    $ tsuru unbind mysql_instance --app my_app

tsuru calls your service to unbind an app with a service instance via DELETE on ``/resources/<service-name>/hostname/<app-hostname>`` (please notice that tsuru does not include a trailing slash). Example of request:

.. highlight:: text

::

    DELETE /resources/mysql_instance/hostname/myapp.myhost.com HTTP/1.0
    Content-Length: 0

Your API should return the following HTTP response code with the respective response body:

    * 200: if the app is successfully unbinded from the instance. You don't need to include any content in the response body.
    * 404: if the service instance does not exist. You don't need to include any content in the response body.
    * 500: in case of any failure in the unbind process. Make sure you include an explanation for the failure in the response body.

Destroying an instance
======================

This process begins when a tsuru customer removes an instance of your service via command line tool:

.. highlight:: bash

::

    $ tsuru service-remove mysql_instance

tsuru calls your service to remove an instance of your service via DELETE on ``/resources/<service-name>`` (please notice that tsuru does not include a trailing slash). Example of request:

.. highlight:: text

::

    DELETE /resources/mysql_instance HTTP/1.0
    Content-Length: 0

Your API should return the following HTTP response code with the respective response body:

    * 200: if the service is successfully destroyed. You don’t need to include any content in the response body.
    * 404: if the service instance does not exist. You don’t need to include any content in the response body.
    * 500: in case of any failure in the destroy process. Make sure you include an explanation for the failure in the response body.

Checking the status of an instance
==================================

This process begins when a tsuru customer wants to check the status of an instance via command line tool:

.. highlight:: bash

::

    $ tsuru service-status mysql_instance

tsuru calls your service to check the status of the instance via GET on ``/resources/mysql_instance/status`` (please notice that tsuru does not include a trailing slash). Example of request:

.. highlight:: text

::

    GET /resources/mysql_instance/status HTTP/1.0

Your API should return the following HTTP response code, with the respective response body:

    * 202: the instance is still being provisioned (pending). You don't need to include any content in the response body.
    * 204: the instance is running and ready for connections (running). You don't need to include any content in the response body.
    * 500: the instance is not running, nor ready for connections. Make sure you include the reason why the instance is not running.

Additional info about an instance
=================================

You can add additional info about instances of your service. To do it it's needed to implement the resource below:

.. highlight:: text

::

    GET /resources/mysql_instance HTTP/1.0

Your API should return the following HTTP response code, with the respective body:

    * 404: when your api doesn't have extra info about the service instance. You don't need to include any content in the response body.
    * 200: when your app has an extra info about the service instance. The response body must be a JSON containing a list of fields. A field is composed by two key/value's `label` and `value`:

.. highlight:: text

::

    HTTP/1.1 200 OK
    Content-Type: application/json; charset=UTF-8

    [{"label": "my label", "value": "my value"}, {"label": "myLabel2.0", "value": "my value 2.0"}]
