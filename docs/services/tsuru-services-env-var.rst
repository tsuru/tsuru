.. Copyright 2014 tsuru authors. All rights reserved.
   Use of this source code is governed by a BSD-style
   license that can be found in the LICENSE file.

+++++++++++++++++++++++++++++++++++
TSURU_SERVICES environment variable
+++++++++++++++++++++++++++++++++++

tsuru exports an special environment variable in applications that use
:doc:`services </services/index>`, this variable is named ``TSURU_SERVICES``.
The value of this example is a JSON describing all services instances that the
application uses. Here is an example of the value of this variable:

.. highlight:: json

::

    {
        "mysql": [
          {"instance_name": "mydb",
           "envs": {"DATABASE_NAME": "mydb",
                    "DATABASE_USER": "mydb",
                    "DATABASE_PASSWORD": "secret",
                    "DATABASE_HOST": "mysql.mycompany.com"}
          },
          {"instance_name": "otherdb",
           "envs": {"DATABASE_NAME": "otherdb",
                    "DATABASE_USER": "otherdb",
                    "DATABASE_PASSWORD": "secret",
                    "DATABASE_HOST": "mysql.mycompany.com"}
          }],
        "redis": [
          {"instance_name": "powerredis",
           "envs": {"REDIS_HOST": "remote.redis.company.com:6379"}
          }],
        "mongodb": []
    }

As described in the structure, the value of the environment variable is a JSON
object, where each key represents a service. In the example above, there are
three services: mysql, redis and mongodb. Each service contains a list of
service instances, and each instance have a name and a map of environment
variables.
