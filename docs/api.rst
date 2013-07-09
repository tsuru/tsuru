+++++++++++++
API reference
+++++++++++++

1. Endpoints
============

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

List services
*************

    * Method: GET
    * URI: /services
    * Format: json

Returns 200 in case of success.

Example:

.. highlight:: bash

::

    GET /services HTTP/1.1
    Content-Legth: 67
    {"service": "mongodb", "instances": ["my_nosql", "other-instance"]}

Create a new service
********************

    * Method: POST
    * URI: /services
    * Format: yaml
    * Body: a yaml with the service metadata.

Returns 200 in case of success.
Returns 500 if the yaml is invalid.
Returns 500 if the service name already exists.
Returns 403 if the user is not a member of a team.

Example:

.. highlight:: bash

::

    GET /services HTTP/1.1
    Body:
	`id: some_service
endpoint:
    production: someservice.com`

1.3 Service instances
---------------------

1.x Quotas
----------

Get quota info of an user
*************************

    * Method: GET
    * URI: /quota/<user>
    * Format: json

Returns 200 in case of success, and json with the quota info.

Example:

.. highlight:: bash

::

    GET /quota/wolverine HTTP/1.1
    Content-Legth: 29
    {"items": 10, "available": 2}

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

Create an user
**************

    * Method: POST
    * URI: /users
    * Body: `{"email":"nobody@globo.com","password":"123456"}`

Returns 200 in case of success.
Returns 400 if the json is invalid.
Returns 400 if the email is invalid.
Returns 400 if the password characters length is less than 6 and greater than 50.
Returns 409 if the email already exists.

Example:

.. highlight:: bash

::

    POST /users HTTP/1.1
    Body: `{"email":"nobody@globo.com","password":"123456"}`

Reset password
**************

    * Method: POST
    * URI: /users/<email>/password?token=token

Returns 200 in case of success.
Returns 404 if the user is not found.

The token parameter is optional.

Example:

.. highlight:: bash

::

    POST /users/user@email.com/password?token=1234 HTTP/1.1

Login
******

    * Method: POST
    * URI: /users/<email>/tokens
    * Body: `{"password":"123456"}`

Returns 200 in case of success.
Returns 400 if the json is invalid.
Returns 400 if the password is empty or nil.
Returns 404 if the user is not found.

Example:

.. highlight:: bash

::

    POST /users/user@email.com/tokens HTTP/1.1

Logout
******

    * Method: DELETE
    * URI: /users/tokens

Returns 200 in case of success.

Example:

.. highlight:: bash

::

    DELETE /users/tokens HTTP/1.1

Change password
***************

    * Method: PUT
    * URI: /users/password
    * Body: `{"old":"123456","new":"654321"}`

Returns 200 in case of success.
Returns 400 if the json is invalid.
Returns 400 if the old or new password is empty or nil.
Returns 400 if the new password characters length is less than 6 and greater than 50.
Returns 403 if the old password does not match with the current password.

Example:

.. highlight:: bash

::

    PUT /users/password HTTP/1.1
    Body: `{"old":"123456","new":"654321"}`

Remove an user
**************

    * Method: DELETE
    * URI: /users

Returns 200 in case of success.

Example:

.. highlight:: bash

::

    DELETE /users HTTP/1.1

Add public key to user
**********************

    * Method: POST
    * URI: /users/keys
    * Body: `{"key":"my-key"}`

Returns 200 in case of success.

Example:

.. highlight:: bash

::

    POST /users/keys HTTP/1.1
    Body: `{"key":"my-key"}`

Remove public key from user
***************************

    * Method: DELETE
    * URI: /users/keys
    * Body: `{"key":"my-key"}`

Returns 200 in case of success.

Example:

.. highlight:: bash

::

    DELETE /users/keys HTTP/1.1
    Body: `{"key":"my-key"}`

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

Info about a team
*****************

    * Method: GET
    * URI: /teams/<teamname>
    * Format: json

Returns 200 in case of success, and json in the body with the info about a team.

Example:

.. highlight:: bash

::

    GET /teams/teamname HTTP/1.1
    {"name": "teamname", "users": ["user@email.com"]}

Add a team
**********

    * Method: POST
    * URI: /teams

Returns 200 in case of success.

Example:

.. highlight:: bash

::

    POST /teams HTTP/1.1

Remove a team
*************

    * Method: DELETE
    * URI: /teams/<teamname>

Returns 200 in case of success.

Example:

.. highlight:: bash

::

    DELELE /teams/myteam HTTP/1.1

Add user to team
****************

    * Method: PUT
    * URI: /teams/<teanmaname>/<username>

Returns 200 in case of success.

Example:

.. highlight:: bash

::

    PUT /teams/myteam/myuser HTTP/1.1

Remove user from team
*********************

    * Method: DELETE
    * URI: /teams/<teanmaname>/<username>

Returns 200 in case of success.

Example:

.. highlight:: bash

::

    DELETE /teams/myteam/myuser HTTP/1.1

1.x Tokens
----------

Generate app token
******************

    * Method: POST
    * URI: /tokens
    * Format: json

Returns 200 in case of success, with the token in the body.

Example:

.. highlight:: bash

::

    POST /tokens HTTP/1.1
	{
		"Token": "sometoken",
		"Creation": "2001/01/01",
		"Expires": 1000,
		"AppName": "appname",
	}
