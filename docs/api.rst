+++++++++++++
Api reference
+++++++++++++

App list:
=========

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
