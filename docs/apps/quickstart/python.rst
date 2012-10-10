.. Copyright 2012 tsuru authors. All rights reserved.
   Use of this source code is governed by a BSD-style
   license that can be found in the LICENSE file.

++++++++++++++++++++++++++++++++++++++
Deploying Python applications in tsuru
++++++++++++++++++++++++++++++++++++++

Overview
========

This document is a hands-on guide to deploying a simple Python application in
Tsuru. The example application will be a very simple Django project associated
to a MySQL service.

Creating the app within tsuru
=============================

To create an app, you use `app-create
<http://go.pkgdoc.org/github.com/globocom/tsuru/cmd/tsuru#Create_an_app>`_
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

You can see all your applications using `app-list
<http://go.pkgdoc.org/github.com/globocom/tsuru/cmd/tsuru#List_apps_that_you_have_access_to>`_
command:

.. highlight:: bash

::

    $ tsuru app-list
    +-------------+---------+----+
    | Application | State   | Ip |
    +-------------+---------+----+
    | blog        | pending |    |
    +-------------+---------+----+

Once your app is ready, it will be displayed as "started" (along with its IP address or public host):

.. highlight:: bash

::

    $ tsuru app-list
    +-------------+---------+-------------+
    | Application | State   | Ip          |
    +-------------+---------+-------------+
    | blog        | started | 10.20.10.20 |
    +-------------+---------+-------------+

Application code
================

This document will not focus on how to write a Django blog, you can clone the
entire source direct from Github:
https://github.com/globocom/tsuru-django-sample. Here is what we did for the
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

When you create a new app, tsuru will display the git remote that you should
use. You can always get it using `app-info
<http://go.pkgdoc.org/github.com/globocom/tsuru/cmd/tsuru#Display_information_about_an_app>`_
command:

.. highlight:: bash

::

    $ tsuru app-info blog
    Application: blog
    State: started
    Repository: git@tsuruhost.com:blog.git
    Platform: python
    Units: 10.20.10.20
    Teams: elasticteam

The git remote will be used to deploy your application using git. You can just
push to tsuru remote and your project will be deployed:

.. highlight:: bash

::

    $ git push git@tsuruhost.com:blog.git master
    Initialized empty Git repository in /mnt/repositories/blog.git/
    Counting objects: 75, done.
    Delta compression using up to 4 threads.
    Compressing objects: 100% (70/70), done.
    Writing objects: 100% (75/75), 11.45 KiB, done.
    Total 75 (delta 36), reused 0 (delta 0)
    remote:
    remote:  ---> Tsuru receiving push
    remote:
    remote:  ---> Clonning your code in your machines
    remote: Cloning into '/home/application/current'...
    remote:
    remote:  ---> Parsing app.conf
    remote:
    remote:  ---> Running pre-restart
    remote:
    remote:  ---> Installing dependencies
    #####################################
    #          OMIT (see below)         #
    #####################################
    remote:  ---> Restarting your app
    remote: /home/ubuntu
    remote:
    remote:  ---> Running pos-restart
    remote:
    remote:  ---> Deploy done!
    remote:
    To git@tsuruhost.com:blog.git
       a211fba..bbf5b53  master -> master

If you get a "Permission denied (publickey).", make sure you're member of a
team and have a public key added to tsuru. To add a key, use `key-add
<http://go.pkgdoc.org/github.com/globocom/tsuru/cmd/tsuru#Add_SSH_public_key_to_tsuru_s_git_server>`_
command:

.. highlight:: bash

::

    $ tsuru key-add ~/.ssh/id_rsa.pub

You can use ``git remote add`` to avoid typing the entire remote url every time
you want to push:

.. highlight:: bash

::

    $ git remote add tsuru git@tsuruhost.com:blog.git

Then you can run:

.. highlight:: bash

::

    $ git push tsuru master
    Everything up-to-date

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

You can see the complete output of installing these dependencies above:

.. highlight:: bash

::

    % git push tsuru master
    #####################################
    #                OMIT               #
    #####################################
    remote:  ---> Installing dependencies
    remote: 2012-10-09 20:05:35,256 INFO Connecting to environment...
    remote: 2012-10-09 20:05:36,531 INFO Connected to environment.
    remote: 2012-10-09 20:05:36,629 INFO Connecting to machine 50 at 10.20.10.20
    remote: Reading package lists...
    remote: Building dependency tree...
    remote: Reading state information...
    remote: libmysqlclient-dev is already the newest version.
    remote: The following extra packages will be installed:
    remote:   libexpat1-dev libssl-dev libssl-doc python2.7-dev
    remote: The following NEW packages will be installed:
    remote:   libexpat1-dev libssl-dev libssl-doc python-dev python2.7-dev
    remote: 0 upgraded, 5 newly installed, 0 to remove and 0 not upgraded.
    remote: Need to get 32.3 MB of archives.
    remote: After this operation, 47.8 MB of additional disk space will be used.
    remote: Get:1 http://us-east-1.ec2.archive.ubuntu.com/ubuntu/ precise-updates/main libexpat1-dev amd64 2.0.1-7.2ubuntu1.1 [216 kB]
    remote: Get:2 http://us-east-1.ec2.archive.ubuntu.com/ubuntu/ precise-updates/main libssl-dev amd64 1.0.1-4ubuntu5.5 [1,525 kB]
    remote: Get:3 http://us-east-1.ec2.archive.ubuntu.com/ubuntu/ precise-updates/main libssl-doc all 1.0.1-4ubuntu5.5 [1,034 kB]
    remote: Get:4 http://us-east-1.ec2.archive.ubuntu.com/ubuntu/ precise-updates/main python2.7-dev amd64 2.7.3-0ubuntu3.1 [29.5 MB]
    remote: Get:5 http://us-east-1.ec2.archive.ubuntu.com/ubuntu/ precise/main python-dev amd64 2.7.3-0ubuntu2 [1,088 B]
    remote: debconf: unable to initialize frontend: Dialog
    remote: debconf: (Dialog frontend will not work on a dumb terminal, an emacs shell buffer, or without a controlling terminal.)
    remote: debconf: falling back to frontend: Readline
    remote: debconf: unable to initialize frontend: Readline
    remote: debconf: (This frontend requires a controlling tty.)
    remote: debconf: falling back to frontend: Teletype
    remote: dpkg-preconfigure: unable to re-open stdin:
    remote: Fetched 32.3 MB in 3s (10.1 MB/s)
    remote: Selecting previously unselected package libexpat1-dev.
    remote: (Reading database ... 32858 files and directories currently installed.)
    remote: Unpacking libexpat1-dev (from .../libexpat1-dev_2.0.1-7.2ubuntu1.1_amd64.deb) ...
    remote: Selecting previously unselected package libssl-dev.
    remote: Unpacking libssl-dev (from .../libssl-dev_1.0.1-4ubuntu5.5_amd64.deb) ...
    remote: Selecting previously unselected package libssl-doc.
    remote: Unpacking libssl-doc (from .../libssl-doc_1.0.1-4ubuntu5.5_all.deb) ...
    remote: Selecting previously unselected package python2.7-dev.
    remote: Unpacking python2.7-dev (from .../python2.7-dev_2.7.3-0ubuntu3.1_amd64.deb) ...
    remote: Selecting previously unselected package python-dev.
    remote: Unpacking python-dev (from .../python-dev_2.7.3-0ubuntu2_amd64.deb) ...
    remote: Processing triggers for man-db ...
    remote: debconf: unable to initialize frontend: Dialog
    remote: debconf: (Dialog frontend will not work on a dumb terminal, an emacs shell buffer, or without a controlling terminal.)
    remote: debconf: falling back to frontend: Readline
    remote: debconf: unable to initialize frontend: Readline
    remote: debconf: (This frontend requires a controlling tty.)
    remote: debconf: falling back to frontend: Teletype
    remote: Setting up libexpat1-dev (2.0.1-7.2ubuntu1.1) ...
    remote: Setting up libssl-dev (1.0.1-4ubuntu5.5) ...
    remote: Setting up libssl-doc (1.0.1-4ubuntu5.5) ...
    remote: Setting up python2.7-dev (2.7.3-0ubuntu3.1) ...
    remote: Setting up python-dev (2.7.3-0ubuntu2) ...
    remote: Requirement already satisfied (use --upgrade to upgrade): Django==1.4.1 in /usr/local/lib/python2.7/dist-packages (from -r /home/application/current/requirements.txt (line 1))
    remote: Downloading/unpacking MySQL-python==1.2.3 (from -r /home/application/current/requirements.txt (line 2))
    remote:   Running setup.py egg_info for package MySQL-python
    remote:
    remote:     warning: no files found matching 'MANIFEST'
    remote:     warning: no files found matching 'ChangeLog'
    remote:     warning: no files found matching 'GPL'
    remote: Downloading/unpacking South==0.7.6 (from -r /home/application/current/requirements.txt (line 3))
    remote:   Running setup.py egg_info for package South
    remote:
    remote: Installing collected packages: MySQL-python, South
    remote:   Running setup.py install for MySQL-python
    remote:     building '_mysql' extension
    remote:     gcc -pthread -fno-strict-aliasing -DNDEBUG -g -fwrapv -O2 -Wall -Wstrict-prototypes -fPIC -Dversion_info=(1,2,3,'final',0) -D__version__=1.2.3 -I/usr/include/mysql -I/usr/include/python2.7 -c _mysql.c -o build/temp.linux-x86_64-2.7/_mysql.o -DBIG_JOINS=1 -fno-strict-aliasing -g
    remote:     In file included from _mysql.c:36:0:
    remote:     /usr/include/mysql/my_config.h:422:0: warning: "HAVE_WCSCOLL" redefined [enabled by default]
    remote:     /usr/include/python2.7/pyconfig.h:890:0: note: this is the location of the previous definition
    remote:     gcc -pthread -shared -Wl,-O1 -Wl,-Bsymbolic-functions -Wl,-Bsymbolic-functions -Wl,-z,relro build/temp.linux-x86_64-2.7/_mysql.o -L/usr/lib/x86_64-linux-gnu -lmysqlclient_r -lpthread -lz -lm -lrt -ldl -o build/lib.linux-x86_64-2.7/_mysql.so
    remote:
    remote:     warning: no files found matching 'MANIFEST'
    remote:     warning: no files found matching 'ChangeLog'
    remote:     warning: no files found matching 'GPL'
    remote:   Running setup.py install for South
    remote:
    remote: Successfully installed MySQL-python South
    remote: Cleaning up...
    #####################################
    #                OMIT               #
    #####################################
    To git@tsuruhost.com:blog.git
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

    web: gunicorn -b 0.0.0.0:8080 blog.wsgi

Now that we commit the file and push the changes to tsuru git server, running
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
    remote:  ---> Clonning your code in your machines
    remote: From git://tsuruhost.com/blog
    remote:  * branch            master     -> FETCH_HEAD
    remote: Updating 81e884e..530c528
    remote: Fast-forward
    remote:  Procfile |    2 +-
    remote:  1 file changed, 1 insertion(+), 1 deletion(-)
    remote:
    remote:  ---> Parsing app.conf
    remote:
    remote:  ---> Running pre-restart
    remote:
    remote:  ---> Installing dependencies
    remote: 2012-10-10 13:47:29,999 INFO Connecting to environment...
    remote: 2012-10-10 13:47:31,175 INFO Connected to environment.
    remote: 2012-10-10 13:47:31,255 INFO Connecting to machine 50 at 10.20.10.20
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
    remote: WARNING: python not running.
    remote: /var/lib/tsuru/hooks/start: line 13: gunicorn: command not found
    remote: /home/ubuntu
    remote:
    remote:  ---> Running pos-restart
    remote:
    remote:  ---> Deploy done!
    remote:
    To git@tsuruhost.com:blog.git
       81e884e..530c528  master -> master

Now we get an error: ``gunicorn: command not found``. It means that we need to
add gunicorn to ``requirements.txt`` file:

.. highlight:: bash

::

    $ cat >> requirements.txt
    gunicorn==0.14.6
    ^-D

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
    remote:  ---> Clonning your code in your machines
    remote: From git://ec2-23-22-70-116.compute-1.amazonaws.com/blog
    remote:  * branch            master     -> FETCH_HEAD
    remote: Updating 530c528..542403a
    remote: Fast-forward
    remote:  requirements.txt |    1 +
    remote:  1 file changed, 1 insertion(+)
    [...]
    remote:  ---> Restarting your app
    remote: WARNING: python not running.
    remote: /home/ubuntu
    remote:
    remote:  ---> Running pos-restart
    remote:
    remote:  ---> Deploy done!
    remote:
    To git@tsuruhost.com:blog.git
       530c528..542403a  master -> master

Now that the app is deployed, you can access it from your browser, getting the
IP or host listed in ``app-list`` and opening it in port ``8080``. For example,
in the list below:

.. highlight:: bash

::

    $ tsuru app-list
    +-------------+---------+-------------+
    | Application | State   | Ip          |
    +-------------+---------+-------------+
    | blog        | started | 10.20.10.20 |
    +-------------+---------+-------------+

We can access the admin of the app in the URL http://10.20.10.20:8080/admin/.

Using services
==============

Now that gunicorn is running, we can accesss the application in the browser,
but we get a Django error: `"Can't connect to local MySQL server through socket
'/var/run/mysqld/mysqld.sock' (2)"`. This error means that we can't connect to
MySQL on localhost. That's because we should not connect to MySQL on localhost,
we must use a service. The service workflow can be resumed to two steps:

#. Create a service instance
#. Bind the service instance to the app

But how can I see what services are available? Easy! Use ``service-list``
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
services: "elastic-search" and "mysql", and none instances. To create our MySQL
instance, we should run the ``service-add`` command:

.. highlight:: bash

::

    $ tsuru service-add
    Service successfully added.

Now, if we run ``service-list`` again, we will see our new service-instance in
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

To bind the service instance to the application, we use the ``bind`` command:

.. highlight:: bash

::

    $ tsuru bind blogsql blog
    Instance blogsql successfully binded to the app blog.

    The following environment variables are now available for use in your app:

    - MYSQL_PORT
    - MYSQL_PASSWORD
    - MYSQL_USER
    - MYSQL_HOST
    - MYSQL_DATABASE_NAME

    For more details, please check the documentation for the service, using service-doc command.

As you can see from bind output, we use environment variable to connect to the
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
    remote:  ---> Clonning your code in your machines
    remote: From git://ec2-23-22-70-116.compute-1.amazonaws.com/blog
    remote:  * branch            master     -> FETCH_HEAD
    remote: Updating ab4e706..a780de9
    remote: Fast-forward
    remote:  blog/settings.py |   12 +++++++-----
    remote:  1 file changed, 7 insertions(+), 5 deletions(-)
    remote:
    remote:  ---> Parsing app.conf
    remote:
    remote:  ---> Installing dependencies
    #####################################
    #               OMIT                #
    #####################################
    remote:
    remote:  ---> Running pre-restart
    remote:
    remote:  ---> Restarting your app
    remote: /home/ubuntu
    remote:
    remote:  ---> Running pos-restart
    remote:
    remote:  ---> Deploy done!
    remote:
    To git@ec2-23-22-70-116.compute-1.amazonaws.com:blog.git
       ab4e706..a780de9  master -> master
