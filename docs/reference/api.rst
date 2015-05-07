.. Copyright 2015 tsuru authors. All rights reserved.
   Use of this source code is governed by a BSD-style
   license that can be found in the LICENSE file.

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
    * Endpoint: /apps
    * Format: JSON

Returns 200 in case of success, and JSON in the body of the response containing the app list.

Example:

::

    GET /apps HTTP/1.1
    Content-Length: 82
    [{"Ip":"10.10.10.10","Name":"app1","Units":[{"Name":"app1/0","State":"started"}]}]

Info about an app
*****************

    * Method: GET
    * Endpoint: /apps/<appname>
    * Format: JSON

Returns 200 in case of success, and a JSON in the body of the response containing the app content.

Example:

::

    GET /apps/myapp HTTP/1.1
    Content-Length: 284
    {"Name":"app1","Framework":"php","Repository":"git@git.com:php.git","State":"dead","Units":[{"Ip":"10.10.10.10","Name":"app1/0","State":"started"}, {"Ip":"9.9.9.9","Name":"app1/1","State":"started"},{"Ip":"","Name":"app1/2","State":"pending"}],"Teams":["tsuruteam","crane"],"Pool": "mypool"}

Remove an app
*************

    * Method: DELETE
    * Endpoint: /apps/<appname>

Returns 200 in case of success.

Example:

::

    DELETE /apps/myapp HTTP/1.1

Create an app
*************

    * Method: POST
    * Endpoint: /apps
    * Format: JSON

Returns 200 in case of success, and JSON in the body of the response containing the status and the URL for Git repository.

Example:

::

    POST /apps HTTP/1.1
    {"status":"success", "repository_url":"git@tsuru.mycompany.com:ble.git"}

Restart an app
**************

    * Method: GET
    * Endpoint: /apps/<appname>/restart

Returns 200 in case of success.

Example:

::

    GET /apps/myapp/restart HTTP/1.1

Get app environment variables
*****************************

    * Method: GET
    * Endpoint: /apps/<appname>/env

Returns 200 in case of success, and JSON in the body returning a dictionary with environment names and values.

Example:

::

    GET /apps/myapp/env HTTP/1.1
    [{"name": "DATABASE_HOST", "value": "localhost", "public": true}]

Set an app environment
**********************

    * Method: POST
    * Endpoint: /apps/<appname>/env

Returns 200 in case of success.

Example:

::

    POST /apps/myapp/env HTTP/1.1

Execute a command
**********************

    * Method: POST
    * Endpoint: /apps/<appname>/run?once=true

Returns 200 in case of success.

Where:

* `once` is a boolean and indicates if the command will run just in an
  unit(once=true) or all of them(once=false). This parameter is not required,
  and the default is false.

Example:

::

    POST /apps/myapp/run HTTP/1.1
    ls -la

Remove one or more environment variables from an app
****************************************************

    * Method: DELETE
    * Endpoint: /apps/<appname>/env

Returns 200 in case of success.

Example:

::

    DELETE /apps/myapp/env HTTP/1.1

Swap the address of two apps
****************************

    * Method: PUT
    * Endpoint: /swap?app1=appname&app2=anotherapp

Returns 200 in case of success.

Example:

::

    PUT /swap?app1=myapp&app2=anotherapp

Get the logs of an app
**********************

    * Method: GET
    * Endpoint: /apps/appname/log?lines=10&source=web&unit=abc123

Returns 200 in case of success. Returns 404 if app is not found.

Where:

* `lines` is the number of the log lines. This parameter is required.
* `source` is the source of the log, like `tsuru` (tsuru API) or a process.
* `unit` is the `id` of an unit.

Example:

::

    GET /apps/myapp/log?lines=20&source=web&unit=83535b503c96
    Content-Length: 142
    [{"Date":"2014-09-26T00:26:30.036Z","Message":"Booting worker with pid: 53","Source":"web","AppName":"tsuru-dashboard","Unit":"83535b503c96"}]

List available pools
********************

    * Method: GET
    * Endpoint: /pools

Returns 200 in case of success.

Example:

::

    GET /pools
    [{"Team":"team1","Pools":["pool1","pool2"]},{"Team":"team2","Pools":["pool3"]}]

Change the pool of an app
*************************

    * Method: POST
    * Endpoint: /apps/<appname>/pool

Returns 200 in case of success. Returns 404 if app is not found.

Example:

::

    POST /apps/myapp/pool


1.2 Services
------------

List services
*************

    * Method: GET
    * Endpoint: /services
    * Format: JSON

Returns 200 in case of success.

Example:

::

    GET /services HTTP/1.1
    Content-Length: 67
    {"service": "mongodb", "instances": ["my_nosql", "other-instance"]}

Create a new service
********************

    * Method: POST
    * Endpoint: /services
    * Format: yaml
    * Body: a yaml with the service metadata.

Returns 200 in case of success.
Returns 403 if the user is not a member of a team.
Returns 500 if the yaml is invalid.
Returns 500 if the service name already exists.

Example:

::

    POST /services HTTP/1.1
    id: some_service
    endpoint:
      production: someservice.com

Remove a service
****************

    * Method: DELETE
    * Endpoint: /services/<servicename>

Returns 204 in case of success.
Returns 403 if user has not access to the server.
Returns 403 if service has instances.
Returns 404 if service is not found.

Example:

::

    DELETE /services/mongodb HTTP/1.1

Update a service
********************

    * Method: PUT
    * Endpoint: /services
    * Format: yaml
    * Body: a yaml with the service metadata.

Returns 200 in case of success.
Returns 403 if the user is not a member of a team.
Returns 500 if the yaml is invalid.
Returns 500 if the service name already exists.

Example:

::

    PUT /services HTTP/1.1
    id: some_service
    endpoint:
      production: someservice.com

Get info about a service
************************

    * Method: GET
    * Endpoint: /services/<servicename>
    * Format: JSON

Returns 200 in case of success.
Returns 404 if the service does not exists.

Example:

::

    GET /services/mongodb HTTP/1.1
    [{"Name": "my-mongo", "Teams": ["myteam"], "Apps": ["myapp"], "ServiceName": "mongodb"}]

Get service documentation
*************************

    * Method: GET
    * Endpoint: /services/<servicename>/doc
    * Format: text

Returns 200 in case of success.
Returns 404 if the service does not exists.

Example:

::

    GET /services/mongodb/doc HTTP/1.1
    Mongodb exports the ...

Update service documentation
****************************

    * Method: PUT
    * Endpoint: /services/<servicename>/doc
    * Format: text
    * Body: text with the documentation

Returns 200 in case of success.
Returns 404 if the service does not exists.

Example:

::

    PUT /services/mongodb/doc HTTP/1.1
    Body: Mongodb exports the ...

Grant access to a service
*************************

    * Method: PUT
    * Endpoint: /services/<servicename>/<teamname>

Returns 200 in case of success.
Returns 404 if the service does not exists.

Example:

::

    PUT /services/mongodb/cobrateam HTTP/1.1

Revoke access from a service
****************************

    * Method: DELETE
    * Endpoint: /services/<servicename>/<teamname>

Returns 200 in case of success.
Returns 404 if the service does not exists.

Example:

::

    DELETE /services/mongodb/cobrateam HTTP/1.1

1.3 Service instances
---------------------

Add a new service instance
**************************

    * Method: POST
    * Endpoint: /services/instances
    * Body: `{"name": "mymysql": "service_name": "mysql"}`

Returns 200 in case of success.
Returns 404 if the service does not exists.

Example:

::

    POST /services/instances HTTP/1.1
    {"name": "mymysql": "service_name": "mysql"}

Remove a service instance
*************************

    * Method: DELETE
    * Endpoint: /services/instances/<serviceinstancename>

Returns 200 in case of success.
Returns 404 if the service does not exists.

Example:

::

    DELETE /services/instances/mymysql HTTP/1.1

Bind a service instance to an app
*********************************

    * Method: PUT
    * Endpoint: /services/instances/<serviceinstancename>/<appname>
    * Format: JSON

Returns 200 in case of success, and JSON with the environment variables to be exported
in the app environ.
Returns 403 if the user has not access to the app.
Returns 404 if the application does not exists.
Returns 404 if the service instance does not exists.

Example:

::

    PUT /services/instances/mymysql/myapp HTTP/1.1
    Content-Length: 29
    {"DATABASE_HOST":"localhost"}

Unbind a service instance from an app
*************************************

    * Method: DELETE
    * Endpoint: /services/instances/<serviceinstancename>/<appname>

Returns 200 in case of success.
Returns 403 if the user has not access to the app.
Returns 404 if the application does not exists.
Returns 404 if the service instance does not exists.

Example:

::

    DELETE /services/instances/mymysql/myapp HTTP/1.1

List all services and your instances
************************************

    * Method: GET
    * Endpoint: /services/instances?app=appname
    * Format: JSON

Returns 200 in case of success and a JSON with the service list.

Where:

* `app` is the name an app you want to use as filter. If defined only instances
  bound to this app will be returned. This parameter is optional.

Example:

::

    GET /services/instances HTTP/1.1
    Content-Length: 52
    [{"service": "redis", "instances": ["redis-globo"]}]

Get an info about a service instance
************************************

    * Method: GET
    * Endpoint: /services/instances/<serviceinstancename>
    * Format: JSON

Returns 200 in case of success and a JSON with the service instance data.
Returns 404 if the service instance does not exists.


Example:

::

    GET /services/instances/mymysql HTTP/1.1
    Content-Length: 71
    {"name": "mongo-1", "servicename": "mongodb", "teams": [], "apps": []}

service instance status
***********************

    * Method: GET
    * Endpoint: /services/instances/<serviceinstancename>/status

Returns 200 in case of success.


Example:

::

    GET /services/instances/mymysql/status HTTP/1.1


1.4 Quotas
----------

Get quota info of a user
************************

    * Method: GET
    * Endpoint: /quota/<user>
    * Format: JSON

Returns 200 in case of success, and JSON with the quota info.

Example:

::

    GET /quota/wolverine HTTP/1.1
    Content-Length: 29
    {"items": 10, "available": 2}

1.5 Healers
-----------

List healers
************

    * Method: GET
    * Endpoint: /healers
    * Format: JSON

Returns 200 in case of success, and JSON in the body with a list of healers.

Example:

::

    GET /healers HTTP/1.1
    Content-Length: 35
    [{"app-heal": "http://healer.com"}]

Execute healer
**************

    * Method: GET
    * Endpoint: /healers/<healer>

Returns 200 in case of success.

Example:

::

    GET /healers/app-heal HTTP/1.1

1.6 Platforms
-------------

List platforms
**************

    * Method: GET
    * Endpoint: /platforms
    * Format: JSON

Returns 200 in case of success, and JSON in the body with a list of platforms.

Example:

::

    GET /platforms HTTP/1.1
    Content-Length: 67
    [{Name: "python"},{Name: "java"},{Name: "ruby20"},{Name: "static"}]

1.7 Users
---------

Create a user
*************

    * Method: POST
    * Endpoint: /users
    * Body: `{"email":"nobody@globo.com","password":"123456"}`

Returns 200 in case of success.
Returns 400 if the JSON is invalid.
Returns 400 if the email is invalid.
Returns 400 if the password characters length is less than 6 and greater than 50.
Returns 409 if the email already exists.

Example:

::

    POST /users HTTP/1.1
    Body: `{"email":"nobody@globo.com","password":"123456"}`

Reset password
**************

    * Method: POST
    * Endpoint: /users/<email>/password?token=token

Returns 200 in case of success.
Returns 404 if the user is not found.

The token parameter is optional.

Example:

::

    POST /users/user@email.com/password?token=1234 HTTP/1.1

Login
******

    * Method: POST
    * Endpoint: /users/<email>/tokens
    * Body: `{"password":"123456"}`

Returns 200 in case of success.
Returns 400 if the JSON is invalid.
Returns 400 if the password is empty or nil.
Returns 404 if the user is not found.

Example:

::

    POST /users/user@email.com/tokens HTTP/1.1
    {"token":"e275317394fb099f62b3993fd09e5f23b258d55f"}

Logout
******

    * Method: DELETE
    * Endpoint: /users/tokens

Returns 200 in case of success.

Example:

::

    DELETE /users/tokens HTTP/1.1

Info about the current user
***************************

    * Method: GET
    * Endpoint: /users/info

Returns 200 in case of success, and a JSON with information about the current user.

Example:

::

    GET /users/info HTTP/1.1
    {"Email":"myuser@company.com","Teams":["frontend","backend","sysadmin","full stack"]}

Change password
***************

    * Method: PUT
    * Endpoint: /users/password
    * Body: `{"old":"123456","new":"654321"}`

Returns 200 in case of success.
Returns 400 if the JSON is invalid.
Returns 400 if the old or new password is empty or nil.
Returns 400 if the new password characters length is less than 6 and greater than 50.
Returns 403 if the old password does not match with the current password.

Example:

::

    PUT /users/password HTTP/1.1
    Body: `{"old":"123456","new":"654321"}`

Remove a user
*************

    * Method: DELETE
    * Endpoint: /users

Returns 200 in case of success.

Example:

::

    DELETE /users HTTP/1.1

Add public key to user
**********************

    * Method: POST
    * Endpoint: /users/keys
    * Body: `{"key":"my-key"}`

Returns 200 in case of success.

Example:

::

    POST /users/keys HTTP/1.1
    Body: `{"key":"my-key"}`

Remove public key from user
***************************

    * Method: DELETE
    * Endpoint: /users/keys
    * Body: `{"key":"my-key"}`

Returns 200 in case of success.

Example:

::

    DELETE /users/keys HTTP/1.1
    Body: `{"key":"my-key"}`

Show API key
************
    * Method: GET
    * Endpoint: /users/api-key
    * Format: JSON

Returns 200 in case of success, and JSON in the body with the API key.

Example:

::

    GET /users/api-key HTTP/1.1
    Body: `{"token": "e275317394fb099f62b3993fd09e5f23b258d55f", "users": "user@email.com"}`

Regenerate API key
******************

    * Method: POST
    * Endpoint: /users/api-key

Returns 200 in case of success.

Example:

::

    POST /users/api-key HTTP/1.1

1.8 Teams
---------

List teams
**********

    * Method: GET
    * Endpoint: /teams
    * Format: JSON

Returns 200 in case of success, and JSON in the body with a list of teams.

Example:

::

    GET /teams HTTP/1.1
    Content-Length: 22
    [{"name": "teamname"}]

Info about a team
*****************

    * Method: GET
    * Endpoint: /teams/<teamname>
    * Format: JSON

Returns 200 in case of success, and JSON in the body with the info about a team.

Example:

::

    GET /teams/teamname HTTP/1.1
    {"name": "teamname", "users": ["user@email.com"]}

Add a team
**********

    * Method: POST
    * Endpoint: /teams

Returns 200 in case of success.

Example:

::

    POST /teams HTTP/1.1
    {"name": "teamname"}

Remove a team
*************

    * Method: DELETE
    * Endpoint: /teams/<teamname>

Returns 200 in case of success.

Example:

::

    DELELE /teams/myteam HTTP/1.1

Add user to team
****************

    * Method: PUT
    * Endpoint: /teams/<teanmaname>/<username>

Returns 200 in case of success.

Example:

::

    PUT /teams/myteam/myuser HTTP/1.1

Remove user from team
*********************

    * Method: DELETE
    * Endpoint: /teams/<teanmaname>/<username>

Returns 200 in case of success.

Example:

::

    DELETE /teams/myteam/myuser HTTP/1.1

1.9 Deploy
----------

Deploy list
***********

    * Method: GET
    * Endpoint: /deploys?app=appname&service=servicename
    * Format: JSON

Returns 200 in case of success, and JSON in the body of the response containing the deploy list.

Where:

* `app` is a `app` name.
* `service` is a `service` name.

Example:

::

    GET /deploys HTTP/1.1
    [{"Ip":"10.10.10.10","Name":"app1","Units":[{"Name":"app1/0","State":"started"}]}]
    [{"ID":"543c20a09e7aea60156191c0","App":"myapp","Timestamp":"2013-11-01T00:01:00-02:00","Duration":29955456221322857,"Commit":"","Error":""},{"ID":"543c20a09e7aea60156191c1","App":"yourapp","Timestamp":"2013-11-01T00:00:01-02:00","Duration":29955456221322857,"Commit":"","Error":""}]

Get info about a deploy
***********************

    * Method: GET
    * Format: JSON
    * Endpoint: /deploys/:deployid

Returns 200 in case of success. Returns 404 if deploy is not found.


Example:

.. highlight: bash

::

    GET /deploys/12345
    {"ID":"54ff355c283dbed9868f01fb","App":"tsuru-dashboard","Timestamp":"2015-03-10T15:18:04.301-03:00","Duration":20413970850,"Commit":"","Error":"","Image":"192.168.50.4:3030/tsuru/app-tsuru-dashboard:v2","Log":"[deploy log]","Origin":"app-deploy","CanRollback":false,"RemoveDate":"0001-01-01T00:00:00Z"}


1.10 Metadata
-------------

There is an endpoint to get metadata about tsuru API:

    * Method: GET
    * Endpoint: /info
    * Format: JSON

Returns 200 in case of success, and JSON in the body of the response containing the metadata.

Example:

::

    GET /info HTTP/1.1
    {"version": "1.0"}
