.. Copyright 2016 tsuru authors. All rights reserved.
   Use of this source code is governed by a BSD-style
   license that can be found in the LICENSE file.

++++++++++++++++++++++++++++
Using services with your app
++++++++++++++++++++++++++++

Overview
========

Tsuru provide ways you to use external services, with that you can have a
database, a storage and a lot of other services.


Using
=====

The service workflow can be resumed to two steps:

#. Create a service instance
#. Bind the service instance to the app

To start you have to list all services provided by tsuru:

.. highlight:: bash

::

    $ tsuru service-list
    +----------------+-----------+
    | Services       | Instances |
    +----------------+-----------+
    | elastic-search |           |
    | mysql          |           |
    +----------------+-----------+

The output from ``service-list`` above says that there are two available
services: "elastic-search" and "mysql", and no instances. To create our MySQL
instance, we should run the command `service-instance-add`:

.. highlight:: bash

::

    $ tsuru service-instance-add mysql db_instance
    Service successfully added.

Now, if we run ``service-list`` again, we will see our new service instance in
the list:

.. highlight:: bash

::

    $ tsuru service-list
    +----------------+---------------+
    | Services       | Instances     |
    +----------------+---------------+
    | elastic-search |               |
    | mysql          | db_instance   |
    +----------------+---------------+

To bind the service instance to the application,
we use the command `service-instance-bind`:

.. highlight:: bash

::

    $ tsuru service-instance-bind mysql db_instance -a myapp
    Instance blogsql is now bound to the app myapp.

    The following environment variables are now available for use in your app:

    - MYSQL_PORT
    - MYSQL_PASSWORD
    - MYSQL_USER
    - MYSQL_HOST
    - MYSQL_DATABASE_NAME

    For more details, please check the documentation for the service, using service-doc command.

As you can see from bind output, we use environment variables to connect to the
MySQL server. Next step is update your app to use these variables to
connect in the database.

After update it and deploy the new version your app will be able to communicate with service.

More tools
==========

To see more information about a service you should use `service-info <service_name>`:

.. highlight:: bash

::

    $ tsuru service-info mysql
    Info for "mysql"

    Instances
    +-------------+---------+-------+
    | Instances   | Plan    | Apps  |
    +-------------+---------+-------+
    | db_instance | default | myapp |
    +-----------------------+-------+

    Plans
    +---------+------------+
    | Name    | Description|
    +---------+------------+
    | medium  | 2G Memory  |
    | default | 1G Memory  |
    +---------+------------+

After create a new service instance, sometimes it takes a while to be done.
To see the state of a service instance you should use
`service-instance-status <service_name> <service_instance>`:

.. highlight:: bash

::

    $ tsuru service-instance-status mysql db_instance
    Service instance "db_instance" is pending

After `service-instance-status` command return `up` to instance,
you are free to use it with your app.
