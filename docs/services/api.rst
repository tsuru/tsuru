.. Copyright 2012 tsuru authors. All rights reserved.
   Use of this source code is governed by a BSD-style
   license that can be found in the LICENSE file.

++++++++++++
API workflow
++++++++++++

tsuru sends requests to the service API to the following actions:

* create a new instance of the service (``tsuru service-instance-add``)
* update a service instance (``tsuru service-instance-update``)
* bind an app with the service instance (``tsuru service-instance-bind``)
* unbind an app from the service instance (``tsuru service-instance-unbind``)
* destroy the service instance (``tsuru service-instance-remove``)
* check the status of the service instance (``tsuru service-instance-status``)
* display additional info about a service, including instances and available
  plans (``tsuru service-info`` and ``tsuru service-instance-info``)

API Specification
=================

The API specification is available as an OpenAPI v3 specification at 
`SwaggerHub <https://app.swaggerhub.com/apis/tsuru/tsuru-service_api/1.0.0>`_ 
and as a yaml file :download:`here <../reference/service_api.yaml>`.


.. _service_api_flow_authentication:

Authentication
==============

tsuru will authenticate with the service API using HTTP basic authentication.
The user can be username or name of the service, and the password is defined in the
:ref:`service manifest <service_manifest>`.

Content-types
=============

tsuru uses ``application/x-www-form-urlencoded`` in requests and expect
``application/json`` in responses.

Here is an example of a request from tsuru, to the service API:

::

    POST /resources HTTP/1.1
    Host: myserviceapi.com
    User-Agent: Go 1.1 package http
    Content-Length: 38
    Accept: application/json
    Authorization: Basic dXNlcjpwYXNzd29yZA==
    Content-Type: application/x-www-form-urlencoded

    name=myinstance&plan=small&team=myteam

Listing available plans
=======================

tsuru will list the available plans whenever the user issues the command
``service-info``

.. highlight:: bash

::

    $ tsuru service-info mysql

It will display all instances of the service that the user has access to, and
also the list of plans, that tsuru gets from the service API by issuing a GET
on ``/resources/plans``. Example of request:

::

    GET /resources/plans HTTP/1.1
    Host: myserviceapi.com
    User-Agent: Go 1.1 package http
    Accept: application/json
    Authorization: Basic dXNlcjpwYXNzd29yZA==
    Content-Type: application/x-www-form-urlencoded

The API should return the following HTTP response codes with the respective
response body:

    * 200: if the operation has succeeded. The response body should include the
      list of the plans, in JSON format. Each plan contains a "name" and a
      "description". Example of response:

::

    HTTP/1.1 200 OK
    Content-Type: application/json; charset=UTF-8

    [{"name":"small","description":"plan for small instances"},
     {"name":"medium","description":"plan for medium instances"},
     {"name":"huge","description":"plan for huge instances"}]

In case of failure, the service API should return the status 500, explaining
what happened in the response body.

Creating a new instance
=======================

This process begins when a tsuru user creates an instance of the service
via command line tool:

.. highlight:: bash

::

    $ tsuru service-instance-add mysql mysql_instance

tsuru calls the service API to create a new instance via POST on ``/resources``
(please notice that tsuru does not include a trailing slash) with the name,
plan and the team that owns the instance. Example of request:

::

    POST /resources HTTP/1.1
    Host: myserviceapi.com
    Content-Length: 56
    User-Agent: Go 1.1 package http
    Accept: application/json
    Authorization: Basic dXNlcjpwYXNzd29yZA==
    Content-Type: application/x-www-form-urlencoded

    name=mysql_instance&plan=small&team=myteam&user=username

The API should return the following HTTP response codes with the respective
response body:

    * 201: when the instance is successfully created. There's no need to
      include any body, as tsuru doesn't expect to get any content back in case
      of success.
    * 500: in case of any failure in the operation. tsuru expects that the
      service API includes an explanation of the failure in the response body.

Updating a service instance
===========================

This endpoint implementation is optional. The process begins when a tsuru
user updates properties of a service instance via command line tool:

.. highlight:: bash

::

    $ tsuru service-instance-update mysql mysql_instance --description "new-description" --tag "tag1" --tag "tag2" --team-owner "new-team-owner" --plan "new-plan"

tsuru calls the service API to inform the instance update via PUT on ``/resources``
(please notice that tsuru does not include a trailing slash) with the new, updated
fields (description, tags, team owner and plan). Example of request:

::

    PUT /resources/mysql_instance HTTP/1.1
    Host: myserviceapi.com
    Content-Length: 79
    User-Agent: Go 1.1 package http
    Accept: application/json
    Authorization: Basic dXNlcjpwYXNzd29yZA==
    Content-Type: application/x-www-form-urlencoded

    description=new-description&tag=tag1&tag=tag2&team=new-team-owner&plan=new-plan

The API should return the following HTTP response codes with the respective
response body:

    * 200: when the instance is successfully updated. There's no need to
      include any body, as tsuru doesn't expect to get any content back in case
      of success.
    * 404: as this endpoint is optional, a 404 response code from the API is
      ignored by tsuru.
    * 500: in case of any failure in the operation. tsuru expects that the
      service API includes an explanation of the failure in the response body.

Binding an app to a service instance
====================================

This process begins when a tsuru user binds an app to an instance of the
service via command line tool:

.. highlight:: bash

::

    $ tsuru service-instance-bind mysql mysql_instance --app my_app

Now, tsuru services has two bind endpoints:
``/resources/<service-instance-name>/bind`` and
``/resources/<service-instance-name>/bind-app``.
The first endpoint will be called every time an app adds an unit.
This endpoint is a POST with:

    * ``app-host`` the host to which the app is accessible
    * ``app-name`` the name of the app
    * ``unit-host`` the address of the unit

 Example of request:

::

    POST /resources/myinstance/bind HTTP/1.1
    Host: myserviceapi.com
    User-Agent: Go 1.1 package http
    Content-Length: 48
    Accept: application/json
    Authorization: Basic dXNlcjpwYXNzd29yZA==
    Content-Type: application/x-www-form-urlencoded

    app-host=myapp.cloud.tsuru.io&unit-host=10.4.3.2

The second endpoint ``/resources/<service-instance-name>/bind-app`` will be
called once when an app is bound to a service.  This endpoint is a POST with:
    
    * ``app-host`` the host to which the app is accessible
    * ``app-name`` the name of the app

Example of request:

::

    POST /resources/myinstance/bind-app HTTP/1.1
    Host: myserviceapi.com
    User-Agent: Go 1.1 package http
    Content-Length: 48
    Accept: application/json
    Authorization: Basic dXNlcjpwYXNzd29yZA==
    Content-Type: application/x-www-form-urlencoded

    app-host=myapp.cloud.tsuru.io&app-name=myapp

The service API should return the following HTTP response code with the
respective response body:

    * 201: if the app has been successfully bound to the instance. The response
      body must be a JSON containing the environment variables from this
      instance that should be exported in the app in order to connect to the
      instance. If the service does not export any environment variable, it can
      return ``null`` or ``{}`` in the response body. Example of response:

::

    HTTP/1.1 201 CREATED
    Content-Type: application/json; charset=UTF-8

    {"MYSQL_HOST":"10.10.10.10","MYSQL_PORT":3306,
     "MYSQL_USER":"ROOT","MYSQL_PASSWORD":"s3cr3t",
     "MYSQL_DATABASE_NAME":"myapp"}

Status codes for errors in the process:

    * 404: if the service instance does not exist. There's no need to include
      anything in the response body.
    * 412: if the service instance is still being provisioned, and not ready
      for binding yet. The service API may include an explanation of the
      failure in the response body.
    * 500: in case of any failure in the operation. tsuru expects that the
      service API includes an explanation of the failure in the response body.

Unbind an app from a service instance
=====================================

This process begins when a tsuru user unbinds an app from an instance of
the service via command line:

.. highlight:: bash

::

    $ tsuru service-instance-unbind mysql mysql_instance --app my_app

Now, tsuru services has two unbind endpoints:
``/resources/<service-instance-name>/bind`` and
``/resources/<service-instance-name>/bind-app``.
The first endpoint will be called every time an app removes an unit.
This endpoint is a DELETE with app-host and unit-host. Example of request:

::

    DELETE /resources/myinstance/bind HTTP/1.1
    Host: myserviceapi.com
    User-Agent: Go 1.1 package http
    Accept: application/json
    Authorization: Basic dXNlcjpwYXNzd29yZA==
    Content-Type: application/x-www-form-urlencoded

    app-host=myapp.cloud.tsuru.io&unit-host=10.4.3.2

The second endpoint ``/resources/<service-instance-name>/bind-app`` will be
called once when the binding between a service and an application is removed.
This endpoint is a DELETE with app-host. Example of request:

::

    DELETE /resources/myinstance/bind-app HTTP/1.1
    Host: myserviceapi.com
    User-Agent: Go 1.1 package http
    Accept: application/json
    Authorization: Basic dXNlcjpwYXNzd29yZA==
    Content-Type: application/x-www-form-urlencoded

    app-host=myapp.cloud.tsuru.io

The API should return the following HTTP response code with the respective
response body:

    * 200: if the operation has succeed and the app is not bound to the service
      instance anymore. There's no need to include anything in the response
      body.
    * 404: if the service instance does not exist. There's no need to include
      anything in the response body.
    * 500: in case of any failure in the operation. tsuru expects that the
      service API includes an explanation of the failure in the response body.

Removing an instance
====================

This process begins when a tsuru user removes an instance of the service
via command line:

.. highlight:: bash

::

    $ tsuru service-instance-remove mysql mysql_instance -y

tsuru calls the service API to remove the instancevia DELETE on
``/resources/<service-name>`` (please notice that tsuru does not include a
trailing slash). Example of request:

::

    DELETE /resources/myinstance HTTP/1.1
    Host: myserviceapi.com
    User-Agent: Go 1.1 package http
    Accept: application/json
    Authorization: Basic dXNlcjpwYXNzd29yZA==
    Content-Type: application/x-www-form-urlencoded

The API should return the following HTTP response codes with the respective
response body:

    * 200: if the service instance has been successfully removed. There's no
      need to include anything in the response body.
    * 404: if the service instance does not exist. There's no need to include
      anything in the response body.
    * 500: in case of any failure in the operation. tsuru expects that the
      service API includes an explanation of the failure in the response body.

Checking the status of an instance
==================================

This process begins when a tsuru user wants to check the status of an
instance via command line:

.. highlight:: bash

::

    $ tsuru service-instance-status mysql mysql_instance

tsuru calls the service API to check the status of the instance via GET on
``/resources/mysql_instance/status`` (please notice that tsuru does not include
a trailing slash). Example of request:

::

    GET /resources/myinstance/status HTTP/1.1
    Host: myserviceapi.com
    User-Agent: Go 1.1 package http
    Accept: application/json
    Authorization: Basic dXNlcjpwYXNzd29yZA==
    Content-Type: application/x-www-form-urlencoded

The API should return the following HTTP response code, with the respective
response body:

    * 202: the instance is still being provisioned (pending). There's no need
      to include anything in the response body.
    * 204: the instance is running and ready for connections (running).
    * 500: the instance is not running, nor ready for connections. tsuru
      expects an explanation of what happened in the response body.

Additional info about an instance
=================================

When the user run ``tsuru service-info <service>`` or
``tsuru service-instance-info``, tsuru will get informations
from all instances. This is an optional endpoint in the service API. Some
services does not provide any extra information for instances. Example of
request:

::

    GET /resources/myinstance HTTP/1.1
    Host: myserviceapi.com
    User-Agent: Go 1.1 package http
    Accept: application/json
    Authorization: Basic dXNlcjpwYXNzd29yZA==
    Content-Type: application/x-www-form-urlencoded

The API should return the following HTTP response codes:

    * 404: when the API doesn't have extra info about the service instance.
      There's no need to include anything in the response body.
    * 200: when there's extra information of the service instance. The response
      body must be a JSON containing a list of items. Each item is a JSON
      object combosed by a label and a value. Example response:

::

    HTTP/1.1 200 OK
    Content-Type: application/json; charset=UTF-8

    [{"label":"my label","value":"my value"},
     {"label":"myLabel2.0","value":"my value 2.0"}]
