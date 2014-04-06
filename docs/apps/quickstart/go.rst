.. Copyright 2014 tsuru authors. All rights reserved.
   Use of this source code is governed by a BSD-style
   license that can be found in the LICENSE file.

++++++++++++++++++++++++++++++++++
Deploying Go applications in tsuru
++++++++++++++++++++++++++++++++++

Overview
========

This document is a hands-on guide to deploying a simple Go web application in
Tsuru. 

Creating the app within tsuru
=============================

To create an app, you use `app-create
<http://godoc.org/github.com/tsuru/tsuru/cmd/tsuru#hdr-Create_an_app>`_
command:

.. highlight:: bash

::

    $ tsuru app-create <app-name> <app-platform>

For go, the app platform is, guess what, ``go``! Let's be over creative
and develop a hello world tutorial-app, let's call it "helloworld":

.. highlight:: bash

::

    $ tsuru app-create helloworld go

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
    | helloworld  | 0 of 0 units in-service |         | No     |
    +-------------+-------------------------+---------+--------+

Once your app is ready, you will be able to deploy your code, e.g.:

.. highlight:: bash

::

    $ tsuru app-list
    +-------------+-------------------------+-------------+--------+
    | Application | Units State Summary     | Address     | Ready? |
    +-------------+-------------------------+-------------+--------+
    | helloworld  | 0 of 0 units in-service |             | Yes    |
    +-------------+-------------------------+-------------+--------+

Application code
================

A simple web application in go `main.go`:

.. highlight:: bash

::

    package main

    import (
        "fmt"
        "net/http"
    )

    func main() {
        http.HandleFunc("/", hello)
        fmt.Println("listening...")
        err := http.ListenAndServe(":8888", nil)
        if err != nil {
            panic(err)
        }
    }

    func hello(res http.ResponseWriter, req *http.Request) {
        fmt.Fprintln(res, "hello, world")
    }

Git deployment
==============

When you create a new app, tsuru will display the Git remote that you should
use. You can always get it using `app-info
<http://godoc.org/github.com/tsuru/tsuru/cmd/tsuru#hdr-Display_information_about_an_app>`_
command:

.. highlight:: bash

::

    $ tsuru app-info --app blog
    Application: go
    Repository: git@cloud.tsuru.io:blog.git
    Platform: go
    Teams: myteam
    Address:

The git remote will be used to deploy your application using git. You can just
push to tsuru remote and your project will be deployed:

.. highlight:: bash

::

    $  git push git@cloud.tsuru.io:helloworld.git master
    Counting objects: 86, done.
    Delta compression using up to 4 threads.
    Compressing objects: 100% (75/75), done.
    Writing objects: 100% (86/86), 29.75 KiB, done.
    Total 86 (delta 2), reused 0 (delta 0)
    remote: Cloning into '/home/application/current'...
    remote: requirements.apt not found.
    remote: Skipping...
    remote: /home/application/current /
    #####################################
    #          OMIT (see below)         #
    #####################################
    remote:  ---> App will be restarted, please check its log for more details...
    remote:
    To git@cloud.tsuru.io:helloworld.git
    * [new branch]      master -> master

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

    $ git remote add tsuru git@cloud.tsuru.io:helloworld.git

Then you can run:

.. highlight:: bash

::

    $ git push tsuru master
    Everything up-to-date

And you will be also able to omit the ``--app`` flag from now on:

.. highlight:: bash

::

    $ tsuru app-info
    Application: helloworld
    Repository: git@cloud.tsuru.io:blog.git
    Platform: go
    Teams: myteam
    Address: helloworld.cloud.tsuru.io
    Units:
    +--------------+---------+
    | Unit         | State   |
    +--------------+---------+
    | 9e70748f4f25 | started |
    +--------------+---------+

For more details on the ``--app`` flag, see `"Guessing app names"
<http://godoc.org/github.com/tsuru/tsuru/cmd/tsuru#hdr-Guessing_app_names>`_
section of tsuru command documentation.

Running the application
=======================

As you can see, in the deploy output there is a step described as "Restarting
your app". In this step, tsuru will restart your app if it's running, or start
it if it's not. But how does tsuru start an application? That's very simple, it
uses a Procfile (a concept stolen from Foreman). In this Procfile, you describe
how your application should be started. Here is how the Procfile should look like:

.. highlight:: text

::

    web: ./main

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
    To git@cloud.tsuru.io:helloworld.git
    d67c3cd..f2a5d2d  master -> master

Now that the app is deployed, you can access it from your browser, getting the
IP or host listed in ``app-list`` and opening it. For example,
in the list below:

::

    $ tsuru app-list
    +-------------+-------------------------+---------------------+--------+
    | Application | Units State Summary     | Address             | Ready? |
    +-------------+-------------------------+---------------------+--------+
    | helloworld  | 1 of 1 units in-service | blog.cloud.tsuru.io | Yes    |
    +-------------+-------------------------+---------------------+--------+

It's done! Now we have a simple go project deployed on tsuru.

Now we can access your `app` in the URL http://helloworld.cloud.tsuru.io/.

Going further
=============

For more information, you can dig into `tsuru docs <http://docs.tsuru.io>`_, or
read `complete instructions of use for the tsuru command
<http://godoc.org/github.com/tsuru/tsuru/cmd/tsuru>`_.
