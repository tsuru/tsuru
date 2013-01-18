+++++++++++++
Api reference
+++++++++++++

App list
========

Returns the app list.

    * Method: GET
    * URI: /apps
    * Format: json

Returns 200 in case of sucess, and json in the body of the response containing the app list.

Example:

.. highlight:: bash

::

    GET /apps HTTP/1.1
    Content-Legth: 82
    [{"Ip":"10.10.10.10","Name":"app1","Units":[{"Name":"app1/0","State":"started"}]}]

App detail
==========

Returns an app.

    * Method: GET
    * URI: /apps/:appname
    * Format: json

Returns 200 in case o f sucess, ando json in the body of the response containing the app.

Example:

.. highlight:: bash

::

    GET /apps/myapp HTTP/1.1
    Content-Legth: 284
    {"Name":"app1","Framework":"php","Repository":"git@git.com:php.git","State":"dead", "Units":[{"Ip":"10.10.10    .10","Name":"app1/0","State":"started"}, {"Ip":"9.9.9.9","Name":"app1/1","State":"started"}, {"Ip":"","Name":"app1/2","Stat    e":"pending"}],"Teams":["tsuruteam","crane"]}
