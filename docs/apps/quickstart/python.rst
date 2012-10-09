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

To create an app, you use ``app-create`` command:

.. highlight:: bash

::

    $ tsuru app-create <app-name> <app-platform>

For Python, the app platform is, guess what, ``python``! Let's be over creative
and develop a never-developed tutorial-app: a blog, and its name will also be
very creative, let's call it "blog":

.. highlight:: bash

::

    $ tsuru app-create blog python

You can see all your app using ``app-list`` command:

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

This document will not focus on how to write a Django blog, you can clone the entire source direct from Github: https://github.com/globocom/tsuru-django-sample. Here is what we did for the project:

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
use. You can always get it using ``app-info`` command:

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
team and have a public key added to tsuru. To add a key, use ``key-add``
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
    remote: 2012-10-09 20:05:36,629 INFO Connecting to machine 50 at ec2-23-22-196-207.compute-1.amazonaws.com
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
