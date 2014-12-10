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
    Content-Length: 82
    [{"Ip":"10.10.10.10","Name":"app1","Units":[{"Name":"app1/0","State":"started"}]}]

Info about an app
*****************

    * Method: GET
    * URI: /apps/<appname>
    * Format: json

Returns 200 in case of success, and a json in the body of the response containing the app content.

Example:

.. highlight:: bash

::

    GET /apps/myapp HTTP/1.1
    Content-Length: 284
    {"Name":"app1","Framework":"php","Repository":"git@git.com:php.git","State":"dead", "Units":[{"Ip":"10.10.10    .10","Name":"app1/0","State":"started"}, {"Ip":"9.9.9.9","Name":"app1/1","State":"started"}, {"Ip":"","Name":"app1/2","Stat    e":"pending"}],"Teams":["tsuruteam","crane"]}

Remove an app
*************

    * Method: DELETE
    * URI: /apps/<appname>

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

Returns 200 in case of success, and json in the body of the response containing the status and the url for git repository.

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

Get app environment variables
*****************************

    * Method: GET
    * URI: /apps/<appname>/env

Returns 200 in case of success, and json in the body returning a dictionary with environment names and values.

Example:

.. highlight:: bash

::

    GET /apps/myapp/env HTTP/1.1
    [{"name": "DATABASE_HOST", "value": "localhost", "public": true}]

Set an app environment
**********************

    * Method: POST
    * URI: /apps/<appname>/env

Returns 200 in case of success.

Example:

.. highlight:: bash

::

    POST /apps/myapp/env HTTP/1.1

Execute a command
**********************

    * Method: POST
    * URI: /apps/<appname>/run?once=true

Returns 200 in case of success.

Where:

* `once` is a boolean and indicates if the command will run just in an unit(once=true) or all of them(once=false). This parameter is not required, and the default is false.

Example:

.. highlight:: bash

::

    POST /apps/myapp/run HTTP/1.1
    ls -la

CLI Example:

.. highlight:: bash

::

    curl -H "Authorization: bearer $(<~/.tsuru_token)" $(<~/.tsuru_target)/apps/<appname>/run?once=true -d 'ls -la'


Delete an app environment
*************************

    * Method: DELETE
    * URI: /apps/<appname>/env

Returns 200 in case of success.

Example:

.. highlight:: bash

::

    DELETE /apps/myapp/env HTTP/1.1

Swapping two apps
*****************

    * Method: PUT
    * URI: /swap?app1=appname&app2=anotherapp

Returns 200 in case of success.

Example:

.. highlight:: bash

::

    PUT /swap?app1=myapp&app2=anotherapp

Get app log
***********

    * Method: GET
    * URI: /apps/appname/log?lines=10&source=web&unit=abc123

Returns 200 in case of success. Returns 404 if app is not found.

Where:

* `lines` is the number of the log lines. This parameter is required.
* `source` is the source of the log, like `tsuru` (tsuru api) or a process.
* `unit` is the `id` of an unit.

Example:

.. highlight: bash

::

    GET /apps/myapp/log?lines=20&source=web&unit=83535b503c96
    Content-Length: 142
    [{"Date":"2014-09-26T00:26:30.036Z","Message":"Booting worker with pid: 53","Source":"web","AppName":"tsuru-dashboard","Unit":"83535b503c96"}]

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
    Content-Length: 67
    {"service": "mongodb", "instances": ["my_nosql", "other-instance"]}

Create a new service
********************

    * Method: POST
    * URI: /services
    * Format: yaml
    * Body: a yaml with the service metadata.

Returns 200 in case of success.
Returns 403 if the user is not a member of a team.
Returns 500 if the yaml is invalid.
Returns 500 if the service name already exists.

Example:

.. highlight:: bash

::

    POST /services HTTP/1.1
    Body:
	`id: some_service
    endpoint:
        production: someservice.com`

Remove a service
****************

    * Method: DELETE
    * URI: /services/<servicename>

Returns 204 in case of success.
Returns 403 if user has not access to the server.
Returns 403 if service has instances.
Returns 404 if service is not found.

Example:

.. highlight:: bash

::

    DELETE /services/mongodb HTTP/1.1

Update a service
********************

    * Method: PUT
    * URI: /services
    * Format: yaml
    * Body: a yaml with the service metadata.

Returns 200 in case of success.
Returns 403 if the user is not a member of a team.
Returns 500 if the yaml is invalid.
Returns 500 if the service name already exists.

Example:

.. highlight:: bash

::

    PUT /services HTTP/1.1
    Body:
	`id: some_service
    endpoint:
        production: someservice.com`

Get info about a service
************************

    * Method: GET
    * URI: /services/<servicename>
    * Format: json

Returns 200 in case of success.
Returns 404 if the service does not exists.

Example:

.. highlight:: bash

::

    GET /services/mongodb HTTP/1.1
    [{"Name": "my-mongo", "Teams": ["myteam"], "Apps": ["myapp"], "ServiceName": "mongodb"}]

Get service documentation
*************************

    * Method: GET
    * URI: /services/<servicename>/doc
    * Format: text

Returns 200 in case of success.
Returns 404 if the service does not exists.

Example:

.. highlight:: bash

::

    GET /services/mongodb/doc HTTP/1.1
    Mongodb exports the ...

Update service documentation
****************************

    * Method: PUT
    * URI: /services/<servicename>/doc
    * Format: text
    * Body: text with the documentation

Returns 200 in case of success.
Returns 404 if the service does not exists.

Example:

.. highlight:: bash

::

    PUT /services/mongodb/doc HTTP/1.1
    Body: Mongodb exports the ...

Grant access to a service
*************************

    * Method: PUT
    * URI: /services/<servicename>/<teamname>

Returns 200 in case of success.
Returns 404 if the service does not exists.

Example:

.. highlight:: bash

::

    PUT /services/mongodb/cobrateam HTTP/1.1

Revoke access from a service
****************************

    * Method: DELETE
    * URI: /services/<servicename>/<teamname>

Returns 200 in case of success.
Returns 404 if the service does not exists.

Example:

.. highlight:: bash

::

    DELETE /services/mongodb/cobrateam HTTP/1.1

1.3 Service instances
---------------------

Add a new service instance
**************************

    * Method: POST
    * URI: /services/instances
    * Body: `{"name": "mymysql": "service_name": "mysql"}`

Returns 200 in case of success.
Returns 404 if the service does not exists.

Example:

.. highlight:: bash

::

    POST /services/instances HTTP/1.1
    {"name": "mymysql": "service_name": "mysql"}

Remove a service instance
*************************

    * Method: DELETE
    * URI: /services/instances/<serviceinstancename>

Returns 200 in case of success.
Returns 404 if the service does not exists.

Example:

.. highlight:: bash

::

    DELETE /services/instances/mymysql HTTP/1.1

Bind a service instance with an app
***********************************

    * Method: PUT
    * URI: /services/instances/<serviceinstancename>/<appname>
    * Format: json

Returns 200 in case of success, and json with the environment variables to be exported
in the app environ.
Returns 403 if the user has not access to the app.
Returns 404 if the application does not exists.
Returns 404 if the service instance does not exists.

Example:

.. highlight:: bash

::

    PUT /services/instances/mymysql/myapp HTTP/1.1
    Content-Length: 29
    {"DATABASE_HOST":"localhost"}

Unbind a service instance with an app
*************************************

    * Method: DELETE
    * URI: /services/instances/<serviceinstancename>/<appname>

Returns 200 in case of success.
Returns 403 if the user has not access to the app.
Returns 404 if the application does not exists.
Returns 404 if the service instance does not exists.

Example:

.. highlight:: bash

::

    DELETE /services/instances/mymysql/myapp HTTP/1.1

List all services and your instances
************************************

    * Method: GET
    * URI: /services/instances?app=appname
    * Format: json

Returns 200 in case of success and a json with the service list.

Where:

* `app` is the name an app you want to use as filter. If defined only instances
  binded to this app will be returned. This parameter is optional.

Example:

.. highlight:: bash

::

    GET /services/instances HTTP/1.1
    Content-Length: 52
    [{"service": "redis", "instances": ["redis-globo"]}]

Get an info about a service instance
************************************

    * Method: GET
    * URI: /services/instances/<serviceinstancename>
    * Format: json

Returns 200 in case of success and a json with the service instance data.
Returns 404 if the service instance does not exists.


Example:

.. highlight:: bash

::

    GET /services/instances/mymysql HTTP/1.1
    Content-Length: 71
    {"name": "mongo-1", "servicename": "mongodb", "teams": [], "apps": []}

service instance status
***********************

    * Method: GET
    * URI: /services/instances/<serviceinstancename>/status

Returns 200 in case of success.


Example:

.. highlight:: bash

::

    GET /services/instances/mymysql/status HTTP/1.1


1.4 Quotas
----------

Get quota info of a user
************************

    * Method: GET
    * URI: /quota/<user>
    * Format: json

Returns 200 in case of success, and json with the quota info.

Example:

.. highlight:: bash

::

    GET /quota/wolverine HTTP/1.1
    Content-Length: 29
    {"items": 10, "available": 2}

1.5 Healers
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
    Content-Length: 35
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

1.6 Platforms
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
    Content-Length: 67
    [{Name: "python"},{Name: "java"},{Name: "ruby20"},{Name: "static"}]

1.7 Users
---------

Create a user
*************

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
    {"token":"e275317394fb099f62b3993fd09e5f23b258d55f"}

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

Remove a user
*************

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

Show API key
************
    * Method: GET
    * URI: /users/api-key
    * Format: json

Returns 200 in case of success, and json in the body with the API key.

Example:

.. highlight:: bash

::

    GET /users/api-key HTTP/1.1
    Body: `{"token": "e275317394fb099f62b3993fd09e5f23b258d55f", "users": "user@email.com"}`

Regenerate API key
******************

    * Method: POST
    * URI: /users/api-key

Returns 200 in case of success.

Example:

.. highlight:: bash

::

    POST /users/api-key HTTP/1.1

1.8 Teams
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
    Content-Length: 22
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
    {"name": "teamname"}

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

1.9 Tokens
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

1.10 Deploy
-----------

Deploy list
***********

    * Method: GET
    * URI: /deploys?app=appname&service=servicename
    * Format: json

Returns 200 in case of success, and json in the body of the response containing the deploy list.

Where:

* `app` is a `app` name.
* `service` is a `service` name.

Example:

.. highlight:: bash

::

    GET /deploys HTTP/1.1
    [{"Ip":"10.10.10.10","Name":"app1","Units":[{"Name":"app1/0","State":"started"}]}]
    [{"ID":"543c20a09e7aea60156191c0","App":"myapp","Timestamp":"2013-11-01T00:01:00-02:00","Duration":29955456221322857,"Commit":"","Error":""},{"ID":"543c20a09e7aea60156191c1","App":"yourapp","Timestamp":"2013-11-01T00:00:01-02:00","Duration":29955456221322857,"Commit":"","Error":""}]

Get info about a deploy
***********************

    * Method: GET
    * Format: json
    * URI: /deploys/:deployid

Returns 200 in case of success. Returns 404 if deploy is not found.


Example:

.. highlight: bash

::

    GET /deploys/12345
    {"App":"myapp","Commit":"e82nn93nd93mm12o2ueh83dhbd3iu112","Diff":"test_diff","Duration":10000000000,"Error":"","Id":"543c201d9e7aea6015618e9d","Timestamp":"2014-10-13T15:55:25-03:00"}
