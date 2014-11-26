.. Copyright 2014 tsuru authors. All rights reserved.
   Use of this source code is governed by a BSD-style
   license that can be found in the LICENSE file.

++++++++++++++++++++++++++++++++++++
Deploying Ruby applications in tsuru
++++++++++++++++++++++++++++++++++++

Overview
========

This document is a hands-on guide to deploying a simple Ruby application in
tsuru. The example application will be a very simple Rails project associated
to a MySQL service.

Creating the app within tsuru
=============================

To create an app, you use `app-create
<http://godoc.org/github.com/tsuru/tsuru-client/tsuru#hdr-Create_an_app>`_
command:

.. highlight:: bash

::

    $ tsuru app-create <app-name> <app-platform>

For Ruby, the app platform is, guess what, ``ruby``! Let's be over creative
and develop a never-developed tutorial-app: a blog, and its name will also be
very creative, let's call it "blog":

.. highlight:: bash

::

    $ tsuru app-create blog ruby

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
    | blog        | 0 of 0 units in-service |             | Yes    |
    +-------------+-------------------------+-------------+--------+

Application code
================

This document will not focus on how to write a blog with Rails, you can clone the
entire source direct from GitHub:
https://github.com/tsuru/tsuru-ruby-sample. Here is what we did for the
project:

#. Create the project (``rails new blog``)
#. Generate the scaffold for Post (``rails generate scaffold Post title:string body:text``)

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
    Repository: git@cloud.tsuru.io:blog.git
    Platform: ruby
    Teams: tsuruteam
    Address:

The git remote will be used to deploy your application using git. You can just
push to tsuru remote and your project will be deployed:

.. highlight:: bash

::

    $  git push git@cloud.tsuru.io:blog.git master
    Counting objects: 86, done.
    Delta compression using up to 4 threads.
    Compressing objects: 100% (75/75), done.
    Writing objects: 100% (86/86), 29.75 KiB, done.
    Total 86 (delta 2), reused 0 (delta 0)
    remote: Cloning into '/home/application/current'...
    remote: requirements.apt not found.
    remote: Skipping...
    remote: /home/application/current /
    remote: Fetching gem metadata from https://rubygems.org/.........
    remote: Fetching gem metadata from https://rubygems.org/..
    #####################################
    #          OMIT (see below)         #
    #####################################
    remote:  ---> App will be restarted, please check its log for more details...
    remote:
    To git@cloud.tsuru.io:blog.git
     * [new branch]      master -> master

If you get a "Permission denied (publickey).", make sure you're member of a
team and have a public key added to tsuru. To add a key, use `key-add
<http://godoc.org/github.com/tsuru/tsuru-client/tsuru#hdr-Add_SSH_public_key_to_tsuru_s_git_server>`_
command:

.. highlight:: bash

::

    $ tsuru key-add ~/.ssh/id_rsa.pub

You can use ``git remote add`` to avoid typing the entire remote url every time
you want to push:

.. highlight:: bash

::

    $ git remote add tsuru git@cloud.tsuru.io:blog.git

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
    Repository: git@cloud.tsuru.io:blog.git
    Platform: ruby
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
  platform/language (in Ruby, ``bundler``).

All ``apt-get`` dependencies must be specified in a ``requirements.apt`` file,
located in the root of your application, and ruby dependencies must be located
in a file called ``Gemfile``, also in the root of the application.
Since we will use MySQL with Rails, we need to install ``mysql``
package using ``gem``, and this package depends on an ``apt-get`` package:
``libmysqlclient-dev``, so here is how ``requirements.apt``
looks like:

.. highlight:: text

::

    libmysqlclient-dev

And here is ``Gemfile``:

.. highlight:: text

::

    source 'https://rubygems.org'

    gem 'rails', '3.2.13'
    gem 'mysql'
    gem 'sass-rails',   '~> 3.2.3'
    gem 'coffee-rails', '~> 3.2.1'
    gem 'therubyracer', :platforms => :ruby
    gem 'uglifier', '>= 1.0.3'
    gem 'jquery-rails'

You can see the complete output of installing these dependencies bellow:

.. highlight:: bash

::

    $ git push tsuru master
    #####################################
    #                OMIT               #
    #####################################
    remote: Reading package lists...
    remote: Building dependency tree...
    remote: Reading state information...
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
    remote: Fetched 2360 kB in 2s (1112 kB/s)
    remote: Selecting previously unselected package mysql-common.
    remote: (Reading database ... 41063 files and directories currently installed.)
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
    remote: /home/application/current /
    remote: Fetching gem metadata from https://rubygems.org/..........
    remote: Fetching gem metadata from https://rubygems.org/..
    remote: Using rake (10.1.0)
    remote: Using i18n (0.6.1)
    remote: Using multi_json (1.7.8)
    remote: Using activesupport (3.2.13)
    remote: Using builder (3.0.4)
    remote: Using activemodel (3.2.13)
    remote: Using erubis (2.7.0)
    remote: Using journey (1.0.4)
    remote: Using rack (1.4.5)
    remote: Using rack-cache (1.2)
    remote: Using rack-test (0.6.2)
    remote: Using hike (1.2.3)
    remote: Using tilt (1.4.1)
    remote: Using sprockets (2.2.2)
    remote: Using actionpack (3.2.13)
    remote: Using mime-types (1.23)
    remote: Using polyglot (0.3.3)
    remote: Using treetop (1.4.14)
    remote: Using mail (2.5.4)
    remote: Using actionmailer (3.2.13)
    remote: Using arel (3.0.2)
    remote: Using tzinfo (0.3.37)
    remote: Using activerecord (3.2.13)
    remote: Using activeresource (3.2.13)
    remote: Using coffee-script-source (1.6.3)
    remote: Using execjs (1.4.0)
    remote: Using coffee-script (2.2.0)
    remote: Using rack-ssl (1.3.3)
    remote: Using json (1.8.0)
    remote: Using rdoc (3.12.2)
    remote: Using thor (0.18.1)
    remote: Using railties (3.2.13)
    remote: Using coffee-rails (3.2.2)
    remote: Using jquery-rails (3.0.4)
    remote: Installing libv8 (3.11.8.17)
    remote: Installing mysql (2.9.1)
    remote: Using bundler (1.3.5)
    remote: Using rails (3.2.13)
    remote: Installing ref (1.0.5)
    remote: Using sass (3.2.10)
    remote: Using sass-rails (3.2.6)
    remote: Installing therubyracer (0.11.4)
    remote: Installing uglifier (2.1.2)
    remote: Your bundle is complete!
    remote: Gems in the groups test and development were not installed.
    remote: It was installed into ./vendor/bundle
    #####################################
    #                OMIT               #
    #####################################
    To git@cloud.tsuru.io:blog.git
       9515685..d67c3cd  master -> master

Running the application
=======================

As you can see, in the deploy output there is a step described as "Restarting
your app". In this step, tsuru will restart your app if it's running, or start
it if it's not. But how does tsuru start an application? That's very simple, it
uses a Procfile (a concept stolen from Foreman). In this Procfile, you describe
how your application should be started. Here is how the Procfile should look like:

.. highlight:: text

::

    web: bundle exec rails server -p $PORT -e production

Now we commit the file and push the changes to tsuru git server, running
another deploy:

.. highlight:: bash

::

    $ git add Procfile
    $ git commit -m "Procfile: added file"
    $ git push tsuru master
    #####################################
    #                OMIT               #
    #####################################
    remote:  ---> App will be restarted, please check its log for more details...
    remote:
    To git@cloud.tsuru.io:blog.git
       d67c3cd..f2a5d2d  master -> master

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

Now that your app is not running with success because the rails can't connect to
MySQL. That's because we add a relation between your rails app and a mysql instance.
To do it we must use a service. The service workflow can be resumed to two steps:

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
MySQL server. Next step is update ``conf/database.yml`` to use these variables to
connect in the database:

.. highlight:: yaml

::

    production:
      adapter: mysql
      encoding: utf8
      database: <%= ENV["MYSQL_DATABASE_NAME"] %>
      pool: 5
      username: <%= ENV["MYSQL_USER"] %>
      password: <%= ENV["MYSQL_PASSWORD"] %>
      host: <%= ENV["MYSQL_HOST"] %>

Now let's commit it and run another deploy:

.. highlight:: bash

::

    $ git add conf/database.yml
    $ git commit -m "database.yml: using environment variables to connect to MySQL"
    $ git push tsuru master
    Counting objects: 7, done.
    Delta compression using up to 4 threads.
    Compressing objects: 100% (4/4), done.
    Writing objects: 100% (4/4), 535 bytes, done.
    Total 4 (delta 3), reused 0 (delta 0)
    remote:
    remote:  ---> tsuru receiving push
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
    To git@cloud.tsuru.io:blog.git
       ab4e706..a780de9  master -> master

Now if we try to access the admin again, we will get another error: `"Table
'blogsql.django_session' doesn't exist"`. Well, that means that we have access
to the database, so bind worked, but we did not set up the database yet. We
need to run ``rake db:migrate`` in the remote server. We can use `run
<http://godoc.org/github.com/tsuru/tsuru-client/tsuru#hdr-Run_an_arbitrary_command_in_the_app_machine>`_
command to execute commands in the machine, so for running ``rake db:migrate`` we could
write:

.. highlight:: bash

::

    $ tsuru app-run -- RAILS_ENV=production bundle exec rake db:migrate
    ==  CreatePosts: migrating ====================================================
    -- create_table(:posts)
       -> 0.1126s
    ==  CreatePosts: migrated (0.1128s) ===========================================

Deployment hooks
================

It would be boring to manually run ``rake db:migrate`` after every deployment.
So we can configure an automatic hook to always run before or after
the app restarts.

tsuru parses a file called ``tsuru.yaml`` and runs restart hooks. As the
extension suggests, this is a YAML file, that contains a list of commands that
should run before and after the restart. Here is our example of tsuru.yaml:

.. highlight:: yaml

::

    hooks:
      restart:
        before-each:
          - RAILS_ENV=production bundle exec rake db:migrate

For more details, check the :ref:`hooks documentation <yaml_deployment_hooks>`.

tsuru will look for the file in the root of the project. Let's commit and
deploy it:

.. highlight:: bash

::

    $ git add tsuru.yaml
    $ git commit -m "tsuru.yaml: added file"
    $ git push tsuru master
    #####################################
    #                OMIT               #
    #####################################
    To git@cloud.tsuru.io:blog.git
       a780de9..1b675b8  master -> master

It is necessary to compile de assets before the app restart. To do it we can
use the ``rake assets:precompile`` command. Then let's add the command to
compile the assets in tsuru.yaml:

.. highlight:: yaml

::

    hooks:
      build:
        - RAILS_ENV=production bundle exec rake assets:precompile

.. highlight:: bash

::

    $ git add tsuru.yaml
    $ git commit -m "tsuru.yaml: added file"
    $ git push tsuru master
    #####################################
    #                OMIT               #
    #####################################
    To git@cloud.tsuru.io:blog.git
       a780de9..1b675b8  master -> master

It's done! Now we have a Rails project deployed on tsuru, using a MySQL
service.

Now we can access your `blog app` in the URL http://blog.cloud.tsuru.io/posts/.

Going further
=============

For more information, you can dig into `tsuru docs <http://docs.tsuru.io>`_, or
read `complete instructions of use for the tsuru command
<http://godoc.org/github.com/tsuru/tsuru-client/tsuru>`_.
