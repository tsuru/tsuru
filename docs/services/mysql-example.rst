.. Copyright 2014 tsuru authors. All rights reserved.
   Use of this source code is governed by a BSD-style
   license that can be found in the LICENSE file.

+++++++++++++++++++++++++++++
HOWTO Install a MySQL service
+++++++++++++++++++++++++++++

First, you must have a `MariaDB server <https://downloads.mariadb.org/mariadb/repositories/>`_, the best "mysql" server in the market. You can also use the standard mysql-server.

.. highlight:: bash

::

    # Ubuntu 13.04
    $ sudo apt-get install software-properties-common
    $ sudo gpg --keyserver hkp://keyserver.ubuntu.com:80 --recv-keys CBCB082A1BB943DB
    $ sudo gpg -a --export CBCB082A1BB943DB | sudo apt-key add -
    $ sudo add-apt-repository 'deb http://mirror.aarnet.edu.au/pub/MariaDB/repo/10.0/ubuntu raring main'
    $ sudo apt-get update
    $ sudo apt-get install mariadb-server

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
    > GRANT ALL PRIVILEGES ON *.* TO 'tsuru'@'%' IDENTIFIED BY 'password' with GRANT OPTION;
    > FLUSH PRIVILEGES;

Now, you will install our mysql-api service example. Just create an application that will be responsible for this service

.. highlight:: bash

::

    # Create a database for this service (change the 192.168.123.131 for your mysql server host)
    $ echo "CREATE DATABASE mysqlapi" | mysql -h 192.168.123.131 -u tsuru -ppassword
    # In a machine with tsuru client and crane installed
    $ git clone https://github.com/globocom/mysqlapi
    # Create the mysqlapi application using python as its platform.
    $ tsuru app-create mysql-api python


In order to have mysql API ready to receive requests, we need some bootstrap stuff.

.. highlight:: bash

::

    #First export the django settings variable:
    $ tsuru env-set --app mysql-api DJANGO_SETTINGS_MODULE=mysqlapi.settings
    # Inject the right environment for that service
    $ tsuru env-set -a mysql-api MYSQLAPI_DB_NAME=mysqlapi
    $ tsuru env-set -a mysql-api MYSQLAPI_DB_USER=tsuru
    $ tsuru env-set -a mysql-api MYSQLAPI_DB_PASSWORD=password
    $ tsuru env-set -a mysql-api MYSQLAPI_DB_HOST=192.168.123.131
    # To show the application's repository
    $ tsuru app-info -a mysql-api|grep Repository
    Repository: git@192.168.123.131:mysql-api.git
    $ git push git@192.168.123.131:mysql-api.git master
    #Now gunicorn is able to run with our wsgi.py configuration. After that, we need to run syncdb:
    $ tsuru run --app mysql-api -- python manage.py syncdb --noinput


To run the API in shared mode, follow this steps


.. highlight:: bash

::

    # First export the needed variables:
    # If the shared mysql database is installed in the same vm that the app is, you can use localhost for MYSQLAPI_SHARED_SERVER
    $ tsuru env-set --app mysql-api MYSQLAPI_SHARED_SERVER=192.168.123.131
    # Here you'll also need to set up a externally accessible endpoint to be used by the apps that are using the service
    $ tsuru env-set --app mysql-api MYSQLAPI_SHARED_SERVER_PUBLIC_HOST=192.168.123.131
    # Here the mysql user to manage the shared databases
    $ tsuru env-set -a mysql-api MYSQLAPI_SHARED_USER=tsuru
    $ tsuru env-set -a mysql-api MYSQLAPI_SHARED_PASSWORD=password

More information about the ways you can work with that api you can found `here <https://github.com/globocom/mysqlapi#choose-your-configuration-mode>`_.

Now you should have your application working. You just need to submit the mysqlapi service via crane.
The manifest.yaml is used by crane to define an id and an endpoint to your service.
For more details, see the text "Services API Workflow": http://docs.tsuru.io/en/latest/services/api.html
To submit your new service, you can run:

.. highlight:: bash

::

    # Configure the service template and point it to the application service (considering that your domain is cloud.company.com)
    $  cat manifest.yaml
    id: mysqlapi
       endpoint:
       production: mysql-api.cloud.company.com
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

Now you can add this service for your applications using the `bind <http://godoc.org/github.com/globocom/tsuru/cmd/tsuru#hdr-Bind_an_application_to_a_service_instance>`_ command

For a complete reference, check the documentation for `crane <http://docs.tsuru.io/en/latest/services/usage.html>`_ command:
`http://godoc.org/github.com/globocom/tsuru/cmd/crane <http://godoc.org/github.com/globocom/tsuru/cmd/crane>`_.
