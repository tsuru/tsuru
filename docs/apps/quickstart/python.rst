.. Copyright 2014 tsuru authors. All rights reserved.
   Use of this source code is governed by a BSD-style
   license that can be found in the LICENSE file.

++++++++++++++++++++++++++++++++++++++
Deploying Python applications in tsuru
++++++++++++++++++++++++++++++++++++++

Overview
========

This document is a hands-on guide to deploying a simple Python application in
Tsuru. The example application will be a very simple Django project associated
to a MySQL service. It's applicable to any WSGI application.

Creating the app within tsuru
=============================

To create an app, you use `app-create
<http://godoc.org/github.com/tsuru/tsuru/cmd/tsuru#hdr-Create_an_app>`_
command:

.. highlight:: bash

::

    $ tsuru app-create <app-name> <app-platform>

For Python, the app platform is, guess what, ``python``! Let's be over creative
and develop a never-developed tutorial-app: a blog, and its name will also be
very creative, let's call it "blog":

.. highlight:: bash

::

    $ tsuru app-create blog python

To list all available platforms, use `platform-list
<http://godoc.org/github.com/tsuru/tsuru/cmd/tsuru#hdr-Display_the_list_of_available_platforms>`_
command.

You can see all your applications using `app-list
<http://godoc.org/github.com/tsuru/tsuru/cmd/tsuru#hdr-List_apps_that_you_have_access_to>`_
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

This document will not focus on how to write a Django blog, you can clone the
entire source direct from GitHub:
https://github.com/tsuru/tsuru-django-sample. Here is what we did for the
project:

#. Create the project (``django-admin.py startproject``)
#. Enable django-admin
#. Install South
#. Create a "posts" app (``django-admin.py startapp posts``)
#. Add a "Post" model to the app
#. Register the model in django-admin
#. Generate the migration using South

Git deployment
==============

When you create a new app, tsuru will display the Git remote that you should
use. You can always get it using `app-info
<http://godoc.org/github.com/tsuru/tsuru/cmd/tsuru#hdr-Display_information_about_an_app>`_
command:

.. highlight:: bash

::

    $ tsuru app-info --app blog
    Application: blog
    Repository: git@git.tsuru.io:blog.git
    Platform: python
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
    remote:  ---> Tsuru receiving push
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
<http://godoc.org/github.com/tsuru/tsuru/cmd/tsuru#hdr-Add_SSH_public_key_to_tsuru_s_git_server>`_
command:

.. highlight:: bash

::

    $ tsuru key-add ~/.ssh/id_rsa.pub

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
    Platform: python
    Teams: tsuruteam
    Address: blog.cloud.tsuru.io
    Units:
    +--------------+---------+
    | Unit         | State   |
    +--------------+---------+
    | 9e70748f4f25 | started |
    +--------------+---------+

For more details on the ``--app`` flag, see `"Guessing app names"
<http://godoc.org/github.com/tsuru/tsuru/cmd/tsuru#hdr-Guessing_app_names>`_
section of tsuru command documentation.

Listing dependencies
====================

In the last section we omitted the dependencies step of deploy. In tsuru, an
application can have two kinds of dependencies:

* **Operating system dependencies**, represented by packages in the package manager
  of the underlying operating system (e.g.: ``yum`` and ``apt-get``);
* **Platform dependencies**, represented by packages in the package manager of the
  platform/language (in Python, ``pip``).

All ``apt-get`` dependencies must be specified in a ``requirements.apt`` file,
located in the root of your application, and pip dependencies must be located
in a file called ``requirements.txt``, also in the root of the application.
Since we will use MySQL with Django, we need to install ``mysql-python``
package using ``pip``, and this package depends on two ``apt-get`` packages:
``python-dev`` and ``libmysqlclient-dev``, so here is how ``requirements.apt``
looks like:

.. highlight:: text

::

    libmysqlclient-dev
    python-dev

And here is ``requirements.txt``:

.. highlight:: text

::

    Django==1.4.1
    MySQL-python==1.2.3
    South==0.7.6

Please notice that we've included ``South`` too, for database migrations, and ``Django``, off-course.

You can see the complete output of installing these dependencies bellow:

.. highlight:: bash

::

    % git push tsuru master
    #####################################
    #                OMIT               #
    #####################################
    remote: Reading package lists...
    remote: Building dependency tree...
    remote: Reading state information...
    remote: python-dev is already the newest version.
    remote: The following extra packages will be installed:
    remote:   libmysqlclient18 mysql-common
    remote: The following NEW packages will be installed:
    remote:   libmysqlclient-dev libmysqlclient18 mysql-common
    remote: 0 upgraded, 3 newly installed, 0 to remove and 0 not upgraded.
    remote: Need to get 2360 kB of archives.
    remote: After this operation, 9289 kB of additional disk space will be used.
    remote: Get:1 http://archive.ubuntu.com/ubuntu/ quantal/main mysql-common all 5.5.27-0ubuntu2 [13.7 kB]
    remote: Get:2 http://archive.ubuntu.com/ubuntu/ quantal/main libmysqlclient18 amd64 5.5.27-0ubuntu2 [949 kB]
    remote: Get:3 http://archive.ubuntu.com/ubuntu/ quantal/main libmysqlclient-dev amd64 5.5.27-0ubuntu2 [1398 kB]
    remote: debconf: unable to initialize frontend: Dialog
    remote: debconf: (Dialog frontend will not work on a dumb terminal, an emacs shell buffer, or without a controlling terminal.)
    remote: debconf: falling back to frontend: Readline
    remote: debconf: unable to initialize frontend: Readline
    remote: debconf: (This frontend requires a controlling tty.)
    remote: debconf: falling back to frontend: Teletype
    remote: dpkg-preconfigure: unable to re-open stdin:
    remote: Fetched 2360 kB in 1s (1285 kB/s)
    remote: Selecting previously unselected package mysql-common.
    remote: (Reading database ... 23143 files and directories currently installed.)
    remote: Unpacking mysql-common (from .../mysql-common_5.5.27-0ubuntu2_all.deb) ...
    remote: Selecting previously unselected package libmysqlclient18:amd64.
    remote: Unpacking libmysqlclient18:amd64 (from .../libmysqlclient18_5.5.27-0ubuntu2_amd64.deb) ...
    remote: Selecting previously unselected package libmysqlclient-dev.
    remote: Unpacking libmysqlclient-dev (from .../libmysqlclient-dev_5.5.27-0ubuntu2_amd64.deb) ...
    remote: Setting up mysql-common (5.5.27-0ubuntu2) ...
    remote: Setting up libmysqlclient18:amd64 (5.5.27-0ubuntu2) ...
    remote: Setting up libmysqlclient-dev (5.5.27-0ubuntu2) ...
    remote: Processing triggers for libc-bin ...
    remote: ldconfig deferred processing now taking place
    remote: sudo: Downloading/unpacking Django==1.4.1 (from -r /home/application/current/requirements.txt (line 1))
    remote:   Running setup.py egg_info for package Django
    remote:
    remote: Downloading/unpacking MySQL-python==1.2.3 (from -r /home/application/current/requirements.txt (line 2))
    remote:   Running setup.py egg_info for package MySQL-python
    remote:
    remote:     warning: no files found matching 'MANIFEST'
    remote:     warning: no files found matching 'ChangeLog'
    remote:     warning: no files found matching 'GPL'
    remote: Downloading/unpacking South==0.7.6 (from -r /home/application/current/requirements.txt (line 3))
    remote:   Running setup.py egg_info for package South
    remote:
    remote: Installing collected packages: Django, MySQL-python, South
    remote:   Running setup.py install for Django
    remote:     changing mode of build/scripts-2.7/django-admin.py from 644 to 755
    remote:
    remote:     changing mode of /usr/local/bin/django-admin.py to 755
    remote:   Running setup.py install for MySQL-python
    remote:     building '_mysql' extension
    remote:     gcc -pthread -fno-strict-aliasing -DNDEBUG -g -fwrapv -O2 -Wall -Wstrict-prototypes -fPIC -Dversion_info=(1,2,3,'final',0) -D__version__=1.2.3 -I/usr/include/mysql -I/usr/include/python2.7 -c _mysql.c -o build/temp.linux-x86_64-2.7/_mysql.o -DBIG_JOINS=1 -fno-strict-aliasing -g
    remote:     In file included from _mysql.c:36:0:
    remote:     /usr/include/mysql/my_config.h:422:0: warning: "HAVE_WCSCOLL" redefined [enabled by default]
    remote:     In file included from /usr/include/python2.7/Python.h:8:0,
    remote:                      from pymemcompat.h:10,
    remote:                      from _mysql.c:29:
    remote:     /usr/include/python2.7/pyconfig.h:890:0: note: this is the location of the previous definition
    remote:     gcc -pthread -shared -Wl,-O1 -Wl,-Bsymbolic-functions -Wl,-Bsymbolic-functions -Wl,-z,relro build/temp.linux-x86_64-2.7/_mysql.o -L/usr/lib/x86_64-linux-gnu -lmysqlclient_r -lpthread -lz -lm -lrt -ldl -o build/lib.linux-x86_64-2.7/_mysql.so
    remote:
    remote:     warning: no files found matching 'MANIFEST'
    remote:     warning: no files found matching 'ChangeLog'
    remote:     warning: no files found matching 'GPL'
    remote:   Running setup.py install for South
    remote:
    remote: Successfully installed Django MySQL-python South
    remote: Cleaning up...
    #####################################
    #                OMIT               #
    #####################################
    To git@git.tsuru.io:blog.git
       a211fba..bbf5b53  master -> master

Running the application
=======================

As you can see, in the deploy output there is a step described as "Restarting
your app". In this step, tsuru will restart your app if it's running, or start
it if it's not. But how does tsuru start an application? That's very simple, it
uses a Procfile (a concept stolen from Foreman). In this Procfile, you describe
how your application should be started. We can use `gunicorn
<http://gunicorn.org/>`_, for example, to start our Django application. Here is
how the Procfile should look like:

.. highlight:: text

::

    web: gunicorn -b 0.0.0.0:$PORT blog.wsgi

Now we commit the file and push the changes to tsuru git server, running
another deploy:

.. highlight:: bash

::

    $ git add Procfile
    $ git commit -m "Procfile: added file"
    $ git push tsuru master
    Counting objects: 5, done.
    Delta compression using up to 4 threads.
    Compressing objects: 100% (2/2), done.
    Writing objects: 100% (3/3), 326 bytes, done.
    Total 3 (delta 1), reused 0 (delta 0)
    remote:
    remote:  ---> Tsuru receiving push
    remote:
    remote:  ---> Installing dependencies
    remote: Reading package lists...
    remote: Building dependency tree...
    remote: Reading state information...
    remote: python-dev is already the newest version.
    remote: libmysqlclient-dev is already the newest version.
    remote: 0 upgraded, 0 newly installed, 0 to remove and 1 not upgraded.
    remote: Requirement already satisfied (use --upgrade to upgrade): Django==1.4.1 in /usr/local/lib/python2.7/dist-packages (from -r /home/application/current/requirements.txt (line 1))
    remote: Requirement already satisfied (use --upgrade to upgrade): MySQL-python==1.2.3 in /usr/local/lib/python2.7/dist-packages (from -r /home/application/current/requirements.txt (line 2))
    remote: Requirement already satisfied (use --upgrade to upgrade): South==0.7.6 in /usr/local/lib/python2.7/dist-packages (from -r /home/application/current/requirements.txt (line 3))
    remote: Cleaning up...
    remote:
    remote:  ---> Restarting your app
    remote: /var/lib/tsuru/hooks/start: line 13: gunicorn: command not found
    remote:
    remote:  ---> Deploy done!
    remote:
    To git@git.tsuru.io:blog.git
       81e884e..530c528  master -> master

Now we get an error: ``gunicorn: command not found``. It means that we need to
add gunicorn to ``requirements.txt`` file:

.. highlight:: bash

::

    $ cat >> requirements.txt
    gunicorn==0.14.6
    ^D

Now we commit the changes and run another deploy:

.. highlight:: bash

::

    $ git add requirements.txt
    $ git commit -m "requirements.txt: added gunicorn"
    $ git push tsuru master
    Counting objects: 5, done.
    Delta compression using up to 4 threads.
    Compressing objects: 100% (3/3), done.
    Writing objects: 100% (3/3), 325 bytes, done.
    Total 3 (delta 1), reused 0 (delta 0)
    remote:
    remote:  ---> Tsuru receiving push
    remote:
    [...]
    remote:  ---> Restarting your app
    remote:
    remote:  ---> Deploy done!
    remote:
    To git@git.tsuru.io:blog.git
       530c528..542403a  master -> master

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


We can access the admin of the app in the URL http://blog.cloud.tsuru.io/admin/.

Using services
==============

Now that gunicorn is running, we can accesss the application in the browser,
but we get a Django error: `"Can't connect to local MySQL server through socket
'/var/run/mysqld/mysqld.sock' (2)"`. This error means that we can't connect to
MySQL on localhost. That's because we should not connect to MySQL on localhost,
we must use a service. The service workflow can be resumed to two steps:

#. Create a service instance
#. Bind the service instance to the app

But how can I see what services are available? Easy! Use `service-list
<http://godoc.org/github.com/tsuru/tsuru/cmd/tsuru#hdr-List_available_services_and_instances>`_
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
<http://godoc.org/github.com/tsuru/tsuru/cmd/tsuru#hdr-Create_a_new_service_instance>`_
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
<http://godoc.org/github.com/tsuru/tsuru/cmd/tsuru#hdr-Bind_an_application_to_a_service_instance>`_
command:

.. highlight:: bash

::

    $ tsuru bind blogsql
    Instance blogsql is now bound to the app blog.

    The following environment variables are now available for use in your app:

    - MYSQL_PORT
    - MYSQL_PASSWORD
    - MYSQL_USER
    - MYSQL_HOST
    - MYSQL_DATABASE_NAME

    For more details, please check the documentation for the service, using service-doc command.

As you can see from bind output, we use environment variables to connect to the
MySQL server. Next step is update ``settings.py`` to use these variables to
connect in the database:

.. highlight:: python

::

    import os

    DATABASES = {
        'default': {
            'ENGINE': 'django.db.backends.mysql',
            'NAME': os.environ.get('MYSQL_DATABASE_NAME', 'blog'),
            'USER': os.environ.get('MYSQL_USER', 'root'),
            'PASSWORD': os.environ.get('MYSQL_PASSWORD', ''),
            'HOST': os.environ.get('MYSQL_HOST', ''),
            'PORT': os.environ.get('MYSQL_PORT', ''),
        }
    }

Now let's commit it and run another deploy:

.. highlight:: bash

::

    $ git add blog/settings.py
    $ git commit -m "settings: using environment variables to connect to MySQL"
    $ git push tsuru master
    Counting objects: 7, done.
    Delta compression using up to 4 threads.
    Compressing objects: 100% (4/4), done.
    Writing objects: 100% (4/4), 535 bytes, done.
    Total 4 (delta 3), reused 0 (delta 0)
    remote:
    remote:  ---> Tsuru receiving push
    remote:
    remote:  ---> Installing dependencies
    #####################################
    #               OMIT                #
    #####################################
    remote:
    remote:  ---> Restarting your app
    remote:
    remote:  ---> Deploy done!
    remote:
    To git@git.tsuru.io:blog.git
       ab4e706..a780de9  master -> master

Now if we try to access the admin again, we will get another error: `"Table
'blogsql.django_session' doesn't exist"`. Well, that means that we have access
to the database, so bind worked, but we did not set up the database yet. We
need to run ``syncdb`` and ``migrate`` (if we're using South) in the remote
server. We can use `run
<http://godoc.org/github.com/tsuru/tsuru/cmd/tsuru#hdr-Run_an_arbitrary_command_in_the_app_machine>`_
command to execute commands in the machine, so for running ``syncdb`` we could
write:

.. highlight:: bash

::

    $ tsuru run -- python manage.py syncdb --noinput
    Syncing...
    Creating tables ...
    Creating table auth_permission
    Creating table auth_group_permissions
    Creating table auth_group
    Creating table auth_user_user_permissions
    Creating table auth_user_groups
    Creating table auth_user
    Creating table django_content_type
    Creating table django_session
    Creating table django_site
    Creating table django_admin_log
    Creating table south_migrationhistory
    Installing custom SQL ...
    Installing indexes ...
    Installed 0 object(s) from 0 fixture(s)

    Synced:
     > django.contrib.auth
     > django.contrib.contenttypes
     > django.contrib.sessions
     > django.contrib.sites
     > django.contrib.messages
     > django.contrib.staticfiles
     > django.contrib.admin
     > south

    Not synced (use migrations):
     - blog.posts
    (use ./manage.py migrate to migrate these)

The same applies for ``migrate``.

Deployment hooks
================

It would be boring to manually run ``syncdb`` and/or ``migrate`` after every
deployment. So we can configure an automatic hook to always run before or after
the app restarts.

Tsuru parses a file called ``app.yaml`` and runs restart hooks. As the
extension suggests, this is a YAML file, that contains a list of commands that
should run before and after the restart. Here is our example of app.yaml:

.. highlight:: yaml

::

    hooks:
      restart:
        after:
          - python manage.py syncdb --noinput
          - python manage.py migrate

For more details, check the :doc:`hooks documentation </apps/deploy-hooks>`.

Tsuru will look for the file in the root of the project. Let's commit and
deploy it:

.. highlight:: bash

::

    $ git add app.yaml
    $ git commit -m "app.yaml: added file"
    $ git push tsuru master
    Counting objects: 4, done.
    Delta compression using up to 4 threads.
    Compressing objects: 100% (3/3), done.
    Writing objects: 100% (3/3), 338 bytes, done.
    Total 3 (delta 1), reused 0 (delta 0)
    remote:
    remote:  ---> Tsuru receiving push
    remote:
    remote:  ---> Installing dependencies
    remote: Reading package lists...
    remote: Building dependency tree...
    remote: Reading state information...
    remote: python-dev is already the newest version.
    remote: libmysqlclient-dev is already the newest version.
    remote: 0 upgraded, 0 newly installed, 0 to remove and 15 not upgraded.
    remote: Requirement already satisfied (use --upgrade to upgrade): Django==1.4.1 in /usr/local/lib/python2.7/dist-packages (from -r /home/application/current/requirements.txt (line 1))
    remote: Requirement already satisfied (use --upgrade to upgrade): MySQL-python==1.2.3 in /usr/local/lib/python2.7/dist-packages (from -r /home/application/current/requirements.txt (line 2))
    remote: Requirement already satisfied (use --upgrade to upgrade): South==0.7.6 in /usr/local/lib/python2.7/dist-packages (from -r /home/application/current/requirements.txt (line 3))
    remote: Requirement already satisfied (use --upgrade to upgrade): gunicorn==0.14.6 in /usr/local/lib/python2.7/dist-packages (from -r /home/application/current/requirements.txt (line 4))
    remote: Cleaning up...
    remote:
    remote:  ---> Restarting your app
    remote:
    remote:  ---> Running restart:after
    remote:
    remote:  ---> Deploy done!
    remote:
    To git@git.tsuru.io:blog.git
       a780de9..1b675b8  master -> master

It's done! Now we have a Django project deployed on tsuru, using a MySQL
service.

Going further
=============

For more information, you can dig into `tsuru docs <http://docs.tsuru.io>`_, or
read `complete instructions of use for the tsuru command
<http://godoc.org/github.com/tsuru/tsuru/cmd/tsuru>`_.
