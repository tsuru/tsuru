.. Copyright 2012 tsuru authors. All rights reserved.
   Use of this source code is governed by a BSD-style
   license that can be found in the LICENSE file.

+++++++++++++++++++++++++++++
HOWTO Install a MySQL service
+++++++++++++++++++++++++++++

First, you must have a MariaDB server, the best mysql server in the market. You also could still using mysql-server

.. highlight:: bash

::

    # Ubuntu 
    $ apt-get install mysql-server -y

.. highlight:: bash

::

    # Centos - creating the mariadb repository 
    $ cat > /etc/yum.repos.d/MariaDB.repo <<END
    # MariaDB 10.0 CentOS repository list - created 2013-09-13 13:25 UTC
    # http://mariadb.org/mariadb/repositories/
    [mariadb]
    name = MariaDB
    baseurl = http://yum.mariadb.org/10.0/centos6-amd64
    gpgkey=https://yum.mariadb.org/RPM-GPG-KEY-MariaDB
    gpgcheck=1
    END
    $ rpm --import https://yum.mariadb.org/RPM-GPG-KEY-MariaDB
    $ yum install MariaDB-server MariaDB-client
    $ service mysql start
    $ chkconfig mysql on

After that, all you need is to create a database admin user for this service, with all necessary grants

.. highlight:: bash

::

    # Creating a database user(log into the database with the root user)
    > GRANT ALL PRIVILEGES ON *.* TO 'tsuru'@'%' IDENTIFIED BY 'password';
    > FLUSH PRIVILEGES;

Now, you will install our mysql-api service example

.. highlight:: bash

::

    # In a machine with tsuru client and crane installed
    $ git clone https://github.com/globocom/mysqlapi
    $ cd mysqlapi
    # Edit the mysqlapi/settings.py, and add the right values for your database. It will look like that:
    $ grep \".*os.*MYSQLAPI_ mysqlapi/settings.py 
    "NAME": os.environ.get("MYSQLAPI_DB_NAME", "mysqlapi"),
    "USER": os.environ.get("MYSQLAPI_DB_USER", "tsuru"),
    "PASSWORD": os.environ.get("MYSQLAPI_DB_PASSWORD", "password"),
    "HOST": os.environ.get("MYSQLAPI_HOST", "192.168.123.131"),
    # Create a database for this service (change the 192.168.123.131 by your mysql server host)
    $ echo "CREATE DATABASE mysqlapi" | mysql -h 192.168.123.131 -u tsuru -ppassword


Create an application that will be responsible for this service

.. highlight:: bash

::

    # Create the mysqlapi application using python as its platform.
    $ tsuru app-create mysql-api python
    # Configure the service template and point it to the application service (considering that your domain is cloud.company.com) 
    $  cat manifest.yaml 
    id: mysqlapi
       endpoint:
       production: mysql-api.cloud.company.com
    # To show the application's repository
    $ tsuru app-info -a mysql-api|grep Repository
    Repository: git@git.cloud.company.com:mysql-api.git
    $ git push git@git.cloud.company.com:mysql-api.git master

In order to have mysql API ready to receive requests, we need some bootstrap stuff.

.. highlight:: bash

::

    #First export the django settings variable:
    $ tsuru env-set --app mysql-api DJANGO_SETTINGS_MODULE=mysqlapi.settings
    #Now gunicorn is able to run with our wsgi.py configuration. After that, we need to run syncdb:
    $ tsuru run --app mysql-api -- python manage.py syncdb --noinput


To run the API in shared mode, follow this steps 


.. highlight:: bash

::

    # First export the needed variables:
    # If the shared mysql database is installed in the sabe vm that the app is, you can use localhost for MYSQLAPI_SHARED_SERVER
    $ tsuru env-set --app mysql-api MYSQLAPI_SHARED_SERVER=192.168.123.131
    # Here you'll also need to set up a externally accessible endpoint to be used by the apps that are using the service
    $ tsuru env-set --app mysql-api MYSQLAPI_SHARED_SERVER_PUBLIC_HOST=192.168.123.131

More information about the ways you can work with that api you can found `here <https://github.com/globocom/mysqlapi#choose-your-configuration-mode>`_
You should have your application working. Now you need to submit the mysql-api service via crane.
The manifest.yaml is used by crane to define an id and an endpoint to your service.
For more details, see the text "Services API Workflow": http://docs.tsuru.io/en/latest/services/api.html
To submit your new service, you can run:

.. highlight:: bash

::
    $ crane create manifest.yaml

To list your services:

.. highlight:: bash

::

    $ crane list
    #OR
    $ tsuru service-list

This will return something like:

.. highlight:: bash

::

    +----------+-----------+
    | Services | Instances |
    +----------+-----------+
    | mysqlapi |           |
    +----------+-----------+


It would be nice if your service had some documentation. To add a documentation to you service you can use:

.. highlight:: bash

::

    $ crane doc-add mysqlapi doc.txt

Crane will read the content of the file and save it.

To show the current documentation of your service:

.. highlight:: bash

::

    $ crane doc-get mysqlapi

doc-get will retrieve the current documentation of the service.


Further instructions
====================

Now you can add this service for your applications using the `bind <http://godoc.org/github.com/globocom/tsuru/cmd/tsuru#hdr-Bind_an_application_to_a_service_instance>` command

For a complete reference, check the documentation for `crane <http://docs.tsuru.io/en/latest/services/usage.html>`_ command:
`<http://godoc.org/github.com/globocom/tsuru/cmd/crane>`_.
