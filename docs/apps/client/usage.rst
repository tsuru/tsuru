++++++++++++
Client usage
++++++++++++

After installing the server, build the cmd/main.go file with the name you wish, and add it to your $PATH. Here we'll call it `tsuru`.
Then you must set the target with your server url, like:

Setting a target
================

.. highlight:: bash

::

    $ tsuru target tsuru.myhost.com

Authentication
==============

After that, all you need is to create a user and authenticate to start creating apps and pushing code to them:

.. highlight:: bash

::

    $ tsuru user-create youremail@gmail.com
    $ tsuru login youremail@gmail.com

Apps
====

Creating an app
---------------

To create an app:

.. highlight:: bash

::

    $ tsuru app-create myblog <platform>

This will return your app's remote url, you should add it to your git repository:

.. highlight:: bash

::

    $ git remote add tsuru git@tsuru.myhost.com:myblog.git

Listing your apps
-----------------

When your app is ready, you can push to it. To check whether it is ready or not, you can use:

.. highlight:: bash

::

    $ tsuru app-list

This will return something like:

.. highlight:: bash

::

    +-------------+---------+--------------+
    | Application | State   | Ip           |
    +-------------+---------+--------------+
    | myblog      | STARTED | 10.10.10.10  |
    +-------------+---------+--------------+

Public Keys
===========

You can try to push now, but you'll get a permission error, because you haven't pushed your key yet.

.. highlight:: bash

::

    $ tsuru key-add

This will search for a `id_rsa.pub` file in ~/.ssh/, if you don't have a generated key yet, you should generate one before running this command.

If you have a public key in other format (for example, DSA), you can also give the public key file to ``key-add``:

.. highlight:: bash

::

    $ tsuru key-add $HOME/.ssh/id_dsa.pub

After your key is added, you can push your application to your cloud:

.. highlight:: bash

::

    $ git push tsuru master

Running commands
================

After that, you can check your app's url in the browser and see your app there. You'll probably need to run migrations or other deploy related commands.
To run a single command, you should use the command line:

.. highlight:: bash

::

    $ tsuru run myblog env/bin/python manage.py syncdb && env/bin/python manage.py migrate

Adding hooks
============

By default, the commands are run from inside the app root directory, which is /home/application. If you have more complicated deploy related commands,
you should use the app.conf pre-restart and pos-restart scripts, those are run before and after the restart of your app, which is triggered everytime you push code.
Below is an app.conf sample:

.. highlight:: yaml

::

    pre-restart:
        deploy/pre.sh
    pos-restart:
        deploy/pos.sh

The app.conf file is located in your app's root directory, and the scripts path in the yaml are relative to it.

Further instructions
====================

For a complete reference, check the documentation for tsuru command:
`<http://go.pkgdoc.org/github.com/timeredbull/tsuru/cmd/tsuru>`_.
