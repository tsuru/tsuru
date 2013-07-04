+++++++++++++
API reference
+++++++++++++

1. Endpoints
===========

1.1 Apps
--------

List apps
*********

    * Method: GET
    * URI: /apps
    * Format: json

Returns 200 in case of success, and json in the body of the response containing the app list.

Example:

.. highlight:: bash

::

    GET /apps HTTP/1.1
    Content-Legth: 82
    [{"Ip":"10.10.10.10","Name":"app1","Units":[{"Name":"app1/0","State":"started"}]}]

Info about an app
*****************

    * Method: GET
    * URI: /apps/:appname
    * Format: json

Returns 200 in case of success, and a json in the body of the response containing the app content.

Example:

.. highlight:: bash

::

    GET /apps/myapp HTTP/1.1
    Content-Legth: 284
    {"Name":"app1","Framework":"php","Repository":"git@git.com:php.git","State":"dead", "Units":[{"Ip":"10.10.10    .10","Name":"app1/0","State":"started"}, {"Ip":"9.9.9.9","Name":"app1/1","State":"started"}, {"Ip":"","Name":"app1/2","Stat    e":"pending"}],"Teams":["tsuruteam","crane"]}

Remove an app
*************

    * Method: DELETE
    * URI: /apps/:appname

Returns 200 in case of success.

Example:

.. highlight:: bash

::

    DELETE /apps/myapp HTTP/1.1

Create an app
*************

    * Method: POST
    * URI: /apps
    * Format: json

Returns 200 in case of success, and json in the body of hte response containing the statusn and the url for git repository.

Example:

.. highlight:: bash

::

    POST /apps HTTP/1.1
    {"status":"success", "repository_url":"git@tsuru.plataformas.glb.com:ble.git"}

Restart an app
**************

    * Method: GET
    * URI: /apps/<appname>/restart

Returns 200 in case of success.

Example:

.. highlight:: bash

::

    GET /apps/myapp/restart HTTP/1.1

Get app enviroment variables
****************************

    * Method: GET
    * URI: /apps/<appname>/env

Returns 200 in case of success, and json in the body returning a dictionary with enviroment names and values..

Example:

.. highlight:: bash

::

    GET /apps/myapp/env HTTP/1.1
    {"DATABASE_HOST":"localhost"}

Set an app enviroment
*********************

    * Method: POST
    * URI: /apps/<appname>/env

Returns 200 in case of success.

Example:

.. highlight:: bash

::

    POST /apps/myapp/env HTTP/1.1

Delete an app enviroment
************************

    * Method: DELETE
    * URI: /apps/<appname>/env

Returns 200 in case of success.

Example:

.. highlight:: bash

::

    DELETE /apps/myapp/env HTTP/1.1

1.2 Services
------------

1.3 Service instances
---------------------

1.x Quotas
----------

1.x Healers
-----------

List healers
************

    * Method: GET
    * URI: /healers
    * Format: json

Returns 200 in case of success, and json in the body with a list of healers.

Example:

.. highlight:: bash

::

    GET /healers HTTP/1.1
    Content-Legth: 35
    [{"app-heal": "http://healer.com"}]

Execute healer
**************

    * Method: GET
    * URI: /healers/<healer>

Returns 200 in case of success.

Example:

.. highlight:: bash

::

    GET /healers/app-heal HTTP/1.1

1.x Platforms
-------------

List platforms
**************

    * Method: GET
    * URI: /platforms
    * Format: json

Returns 200 in case of success, and json in the body with a list of platforms.

Example:

.. highlight:: bash

::

    GET /platforms HTTP/1.1
    Content-Legth: 67
    [{Name: "python"},{Name: "java"},{Name: "ruby20"},{Name: "static"}]

1.x Users
---------

1.x Teams
---------

List teams
**********

    * Method: GET
    * URI: /teams
    * Format: json

Returns 200 in case of success, and json in the body with a list of teams.

Example:

.. highlight:: bash

::

    GET /teams HTTP/1.1
    Content-Legth: 22
    [{"name": "teamname"}]

Add a team
**********

    * Method: POST
    * URI: /teams
    * Format: json

Returns 200 in case of success.

Example:

.. highlight:: bash

::

    POST /teams HTTP/1.1

1.x Tokens
----------
