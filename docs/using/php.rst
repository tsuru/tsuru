.. Copyright 2015 tsuru authors. All rights reserved.
   Use of this source code is governed by a BSD-style
   license that can be found in the LICENSE file.

++++++++++++++++++++++++++++++++++++++
Deploying PHP applications in tsuru
++++++++++++++++++++++++++++++++++++++

Overview
========

This document is a hands-on guide to deploying a simple PHP application in
tsuru. The example application will be a very simple Wordpress project associated
to a MySQL service. It's applicable to any php over apache application.

Creating the app within tsuru
=============================

To create an app, you use `app-create
<http://godoc.org/github.com/tsuru/tsuru-client/tsuru#hdr-Create_an_app>`_
command:

.. highlight:: bash

::

    $ tsuru app-create <app-name> <app-platform>

For PHP, the app platform is, guess what, ``php``! Let's be over creative
and develop a never-developed tutorial-app: a blog, and its name will also be
very creative, let's call it "blog":

.. highlight:: bash

::

    $ tsuru app-create blog php

To list all available platforms, use `platform-list
<http://godoc.org/github.com/tsuru/tsuru-client/tsuru#hdr-Display_the_list_of_available_platforms>`_
command.

You can see all your applications using `app-list
<http://godoc.org/github.com/tsuru/tsuru-client/tsuru#hdr-List_apps_that_you_have_access_to>`_
command:

.. highlight:: bash

::

    $ tsuru app-list
    +-------------+-------------------------+---------+--------+
    | Application | Units State Summary     | Address | Ready? |
    +-------------+-------------------------+---------+--------+
    | blog        | 0 of 0 units in-service |         | No     |
    +-------------+-------------------------+---------+--------+

Once your app is ready, you will be able to deploy your code, e.g.:

.. highlight:: bash

::

    $ tsuru app-list
    +-------------+-------------------------+-------------+--------+
    | Application | Units State Summary     | Address     | Ready? |
    +-------------+-------------------------+-------------+--------+
    | blog        | 0 of 1 units in-service |             | Yes    |
    +-------------+-------------------------+-------------+--------+

Application code
================

This document will not focus on how to write a php blog, you can download the
entire source direct from wordpress:
http://wordpress.org/latest.zip. Here is all you need to do with your
project:

.. highlight:: bash

::

    # Download and unpack wordpress
    $ wget http://wordpress.org/latest.zip
    $ unzip latest.zip
    # Preparing wordpress for tsuru
    $ cd wordpress
    # Notify tsuru about the necessary packages
    $ echo php5-mysql > requirements.apt
    # Preparing the application to receive the tsuru environment related to the mysql service
    $ sed "s/'database_name_here'/getenv('MYSQL_DATABASE_NAME')/; \
                s/'username_here'/getenv('MYSQL_USER')/; \
                s/'localhost'/getenv('MYSQL_HOST')/; \
                s/'password_here'/getenv('MYSQL_PASSWORD')/" \
                wp-config-sample.php  > wp-config.php
    # Creating a local git repository
    $ git init
    $ git add .
    $ git commit -m 'initial project version'


Git deployment
==============

When you create a new app, tsuru will display the Git remote that you should
use. You can always get it using `app-info
<http://godoc.org/github.com/tsuru/tsuru-client/tsuru#hdr-Display_information_about_an_app>`_
command:

.. highlight:: bash

::

    $ tsuru app-info --app blog
    Application: blog
    Repository: git@git.tsuru.io:blog.git
    Platform: php
    Teams: tsuruteam
    Address:

The git remote will be used to deploy your application using git. You can just
push to tsuru remote and your project will be deployed:

.. highlight:: bash

::

    $ git push git@git.tsuru.io:blog.git master
    Counting objects: 119, done.
    Delta compression using up to 4 threads.
    Compressing objects: 100% (53/53), done.
    Writing objects: 100% (119/119), 16.24 KiB, done.
    Total 119 (delta 55), reused 119 (delta 55)
    remote:
    remote:  ---> tsuru receiving push
    remote:
    remote: From git://cloud.tsuru.io/blog.git
    remote:  * branch            master     -> FETCH_HEAD
    remote:
    remote:  ---> Installing dependencies
    #####################################
    #          OMIT (see below)         #
    #####################################
    remote:  ---> Restarting your app
    remote:
    remote:  ---> Deploy done!
    remote:
    To git@git.tsuru.io:blog.git
       a211fba..bbf5b53  master -> master

If you get a "Permission denied (publickey).", make sure you're member of a
team and have a public key added to tsuru. To add a key, use `key-add
<http://godoc.org/github.com/tsuru/tsuru-client/tsuru#hdr-Add_SSH_public_key_to_tsuru_s_git_server>`_
command:

.. highlight:: bash

::

    $ tsuru key-add ~/.ssh/id_dsa.pub

You can use ``git remote add`` to avoid typing the entire remote url every time
you want to push:

.. highlight:: bash

::

    $ git remote add tsuru git@git.tsuru.io:blog.git

Then you can run:

.. highlight:: bash

::

    $ git push tsuru master
    Everything up-to-date

And you will be also able to omit the ``--app`` flag from now on:

.. highlight:: bash

::

    $ tsuru app-info
    Application: blog
    Repository: git@git.tsuru.io:blog.git
    Platform: php
    Teams: tsuruteam
    Address: blog.cloud.tsuru.io
    Units:
    +--------------+---------+
    | Unit         | State   |
    +--------------+---------+
    | 9e70748f4f25 | started |
    +--------------+---------+

For more details on the ``--app`` flag, see `"Guessing app names"
<http://godoc.org/github.com/tsuru/tsuru-client/tsuru#hdr-Guessing_app_names>`_
section of tsuru command documentation.

Listing dependencies
====================

In the last section we omitted the dependencies step of deploy. In tsuru, an
application can have two kinds of dependencies:

* **Operating system dependencies**, represented by packages in the package manager
  of the underlying operating system (e.g.: ``yum`` and ``apt-get``);
* **Platform dependencies**, represented by packages in the package manager of the
  platform/language (e.g. in Python, ``pip``).

All ``apt-get`` dependencies must be specified in a ``requirements.apt`` file,
located in the root of your application, and pip dependencies must be located
in a file called ``requirements.txt``, also in the root of the application.
Since we will use MySQL with PHP, we need to install the package depends on just
one ``apt-get`` package:
``php5-mysql``, so here is how ``requirements.apt``
looks like:

.. highlight:: text

::

    php5-mysql


You can see the complete output of installing these dependencies below:

.. highlight:: bash

::

    % git push tsuru master
    #####################################
    #                OMIT               #
    #####################################
    Counting objects: 1155, done.
    Delta compression using up to 4 threads.
    Compressing objects: 100% (1124/1124), done.
    Writing objects: 100% (1155/1155), 4.01 MiB | 327 KiB/s, done.
    Total 1155 (delta 65), reused 0 (delta 0)
    remote: Cloning into '/home/application/current'...
    remote: Reading package lists...
    remote: Building dependency tree...
    remote: Reading state information...
    remote: The following extra packages will be installed:
    remote:   libmysqlclient18 mysql-common
    remote: The following NEW packages will be installed:
    remote:   libmysqlclient18 mysql-common php5-mysql
    remote: 0 upgraded, 3 newly installed, 0 to remove and 0 not upgraded.
    remote: Need to get 1042 kB of archives.
    remote: After this operation, 3928 kB of additional disk space will be used.
    remote: Get:1 http://archive.ubuntu.com/ubuntu/ quantal/main mysql-common all 5.5.27-0ubuntu2 [13.7 kB]
    remote: Get:2 http://archive.ubuntu.com/ubuntu/ quantal/main libmysqlclient18 amd64 5.5.27-0ubuntu2 [949 kB]
    remote: Get:3 http://archive.ubuntu.com/ubuntu/ quantal/main php5-mysql amd64 5.4.6-1ubuntu1 [79.0 kB]
    remote: Fetched 1042 kB in 1s (739 kB/s)
    remote: Selecting previously unselected package mysql-common.
    remote: (Reading database ... 23874 files and directories currently installed.)
    remote: Unpacking mysql-common (from .../mysql-common_5.5.27-0ubuntu2_all.deb) ...
    remote: Selecting previously unselected package libmysqlclient18:amd64.
    remote: Unpacking libmysqlclient18:amd64 (from .../libmysqlclient18_5.5.27-0ubuntu2_amd64.deb) ...
    remote: Selecting previously unselected package php5-mysql.
    remote: Unpacking php5-mysql (from .../php5-mysql_5.4.6-1ubuntu1_amd64.deb) ...
    remote: Processing triggers for libapache2-mod-php5 ...
    remote:  * Reloading web server config
    remote:    ...done.
    remote: Setting up mysql-common (5.5.27-0ubuntu2) ...
    remote: Setting up libmysqlclient18:amd64 (5.5.27-0ubuntu2) ...
    remote: Setting up php5-mysql (5.4.6-1ubuntu1) ...
    remote: Processing triggers for libc-bin ...
    remote: ldconfig deferred processing now taking place
    remote: Processing triggers for libapache2-mod-php5 ...
    remote:  * Reloading web server config
    remote:    ...done.
    remote: sudo: unable to resolve host 8cf20f4da877
    remote: sudo: unable to resolve host 8cf20f4da877
    remote: debconf: unable to initialize frontend: Dialog
    remote: debconf: (Dialog frontend will not work on a dumb terminal, an emacs shell buffer, or without a controlling terminal.)
    remote: debconf: falling back to frontend: Readline
    remote: debconf: unable to initialize frontend: Dialog
    remote: debconf: (Dialog frontend will not work on a dumb terminal, an emacs shell buffer, or without a controlling terminal.)
    remote: debconf: falling back to frontend: Readline
    remote:
    remote: Creating config file /etc/php5/mods-available/mysql.ini with new version
    remote: debconf: unable to initialize frontend: Dialog
    remote: debconf: (Dialog frontend will not work on a dumb terminal, an emacs shell buffer, or without a controlling terminal.)
    remote: debconf: falling back to frontend: Readline
    remote:
    remote: Creating config file /etc/php5/mods-available/mysqli.ini with new version
    remote: debconf: unable to initialize frontend: Dialog
    remote: debconf: (Dialog frontend will not work on a dumb terminal, an emacs shell buffer, or without a controlling terminal.)
    remote: debconf: falling back to frontend: Readline
    remote:
    remote: Creating config file /etc/php5/mods-available/pdo_mysql.ini with new version
    remote:
    remote:  ---> App will be restarted, please check its log for more details...
    remote:
    To git@git.tsuru.io:ingress.git
     * [new branch]      master -> master


Running the application
=======================

As you can see, in the deploy output there is a step described as "App will be
restarted". In this step, tsuru will restart your app if it's running, or start
it if it's not.
Now that the app is deployed, you can access it from your browser, getting the
IP or host listed in ``app-list`` and opening it. For example,
in the list below:

::

    $ tsuru app-list
    +-------------+-------------------------+---------------------+--------+
    | Application | Units State Summary     | Address             | Ready? |
    +-------------+-------------------------+---------------------+--------+
    | blog        | 1 of 1 units in-service | blog.cloud.tsuru.io | Yes    |
    +-------------+-------------------------+---------------------+--------+


Using services
==============

Now that php is running, we can accesss the application in the browser,
but we get a database connection error: `"Error establishing a database connection"`.
This error means that we can't connect to MySQL. That's because we
should not connect to MySQL on localhost, we must use a service.
The service workflow can be resumed to two steps:

#. Create a service instance
#. Bind the service instance to the app

But how can I see what services are available? Easy! Use `service-list
<http://godoc.org/github.com/tsuru/tsuru-client/tsuru#hdr-List_available_services_and_instances>`_
command:

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
instance, we should run the `service-add
<http://godoc.org/github.com/tsuru/tsuru-client/tsuru#hdr-Create_a_new_service_instance>`_
command:

.. highlight:: bash

::

    $ tsuru service-add mysql blogsql
    Service successfully added.

Now, if we run ``service-list`` again, we will see our new service instance in
the list:

.. highlight:: bash

::

    $ tsuru service-list
    +----------------+-----------+
    | Services       | Instances |
    +----------------+-----------+
    | elastic-search |           |
    | mysql          | blogsql   |
    +----------------+-----------+

To bind the service instance to the application, we use the `bind
<http://godoc.org/github.com/tsuru/tsuru-client/tsuru#hdr-Bind_an_application_to_a_service_instance>`_
command:

.. highlight:: bash

::

    $ tsuru service-bind blogsql
    Instance blogsql is now bound to the app blog.

    The following environment variables are now available for use in your app:

    - MYSQL_PORT
    - MYSQL_PASSWORD
    - MYSQL_USER
    - MYSQL_HOST
    - MYSQL_DATABASE_NAME

    For more details, please check the documentation for the service, using service-doc command.

As you can see from bind output, we use environment variables to connect to the
MySQL server. Next step would be update the ``wp-config.php`` to use these variables to
connect in the database:

.. highlight:: bash

::

    $ grep getenv wp-config.php
    define('DB_NAME', getenv('MYSQL_DATABASE_NAME'));
    define('DB_USER', getenv('MYSQL_USER'));
    define('DB_PASSWORD', getenv('MYSQL_PASSWORD'));
    define('DB_HOST', getenv('MYSQL_HOST'));


You can extend your wordpress installing plugins into your repository. In the example below, we
are adding the Amazon S3 capability to wordpress, just installing 2 more plugins: `Amazon S3 and Cloudfront <http://wordpress.org/plugins/amazon-s3-and-cloudfront>`_ +
`Amazon Web Services <http://wordpress.org/plugins/amazon-web-services>`_. It's the right way to store content files into tsuru.

.. highlight:: bash

::

    $ cd wp-content/plugins/
    $ wget http://downloads.wordpress.org/plugin/amazon-web-services.0.1.zip
    $ wget http://downloads.wordpress.org/plugin/amazon-s3-and-cloudfront.0.6.1.zip
    $ unzip amazon-web-services.0.1.zip
    $ unzip amazon-s3-and-cloudfront.0.6.1.zip
    $ rm -f amazon-web-services.0.1.zip amazon-s3-and-cloudfront.0.6.1.zip
    $ git add amazon-web-services/ amazon-s3-and-cloudfront/

Now you need to add the amazon AWS_ACCESS_KEY_ID and AWS_SECRET_ACCESS_KEY environments
support into wp-config.php. You could add these environments right after the WP_DEBUG as below:

.. highlight:: bash

::

    $ grep -A2 define.*WP_DEBUG  wp-config.php
    define('WP_DEBUG', false);
    define('AWS_ACCESS_KEY_ID', getenv('AWS_ACCESS_KEY_ID'));
    define('AWS_SECRET_ACCESS_KEY', getenv('AWS_SECRET_ACCESS_KEY'));
    $ git add wp-config.php
    $ git commit -m 'adding plugins for S3'
    $ git push tsuru master

Now, just inject the right values for these environments with tsuru env-set as below:

.. highlight:: bash

::

    $ tsuru env-set AWS_ACCESS_KEY_ID="xxx" AWS_SECRET_ACCESS_KEY="xxxxx" -a blog

It's done! Now we have a PHP project deployed on tsuru, with S3 support using a MySQL
service.




Going further
=============

For more information, you can dig into `tsuru docs <http://docs.tsuru.io>`_, or
read `complete instructions of use for the tsuru command
<http://godoc.org/github.com/tsuru/tsuru-client/tsuru>`_.
