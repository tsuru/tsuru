.. Copyright 2014 tsuru authors. All rights reserved.
   Use of this source code is governed by a BSD-style
   license that can be found in the LICENSE file.

++++++++++++
Client usage
++++++++++++

After :doc:`installing the tsuru client </using/install-client>`, you must set the
target with the tsuru server URL, something like:

Setting a target
================

.. highlight:: bash

::

    $ tsuru target-add default https://cloud.tsuru.io
    $ tsuru target-set default

Authentication
==============

After that, all you need is to create a user and authenticate to start creating
apps and pushing code to them. Use `create-user
<http://godoc.org/github.com/tsuru/tsuru-client/tsuru#hdr-Create_a_user>`_ and
`login
<http://godoc.org/github.com/tsuru/tsuru-client/tsuru#hdr-Authenticate_within_remote_tsuru_server>`_:

.. highlight:: bash

::

    $ tsuru user-create youremail@gmail.com
    $ tsuru login youremail@gmail.com

Apps
====

Associating your user to a team
-------------------------------

You need to be member of a team to create an app. To create a new team, use
`create-team
<http://godoc.org/github.com/tsuru/tsuru-client/tsuru#hdr-Create_a_new_team_for_the_user>`_:

.. highlight:: bash

::

    $ tsuru team-create teamname

Creating an app
---------------

To create an app, use `app-create
<http://godoc.org/github.com/tsuru/tsuru-client/tsuru#hdr-Create_an_app>`_:

.. highlight:: bash

::

    $ tsuru app-create myblog <platform>

This will return your app's remote url, you should add it to your git
repository:

.. highlight:: bash

::

    $ git remote add tsuru git@tsuru.myhost.com:myblog.git

Listing your apps
-----------------

When your app is ready, you can push to it. To check whether it is ready or
not, you can use `app-list
<http://godoc.org/github.com/tsuru/tsuru-client/tsuru#hdr-List_apps_that_you_have_access_to>`_:

.. highlight:: bash

::

    $ tsuru app-list

This will return something like:

.. highlight:: bash

::

    +-------------+-------------------------+-------------------------------------------+
    | Application | Units State Summary     | Ip                                        |
    +-------------+-------------------------+-------------------------------------------+
    | myblog      | 1 of 1 units in-service | myblog-838381.us-east-1-elb.amazonaws.com |
    +-------------+-------------------------+-------------------------------------------+

Showing app info
----------------

You can also use the `app-info
<http://godoc.org/github.com/tsuru/tsuru-client/tsuru#hdr-Display_information_about_an_app>`_
command to view information of an app. Including the status of the app:

.. highlight:: bash

::

    $ tsuru app-info

This will return something like:

.. highlight:: bash

::

    Application: myblog
    Platform: gunicorn
    Repository: git@githost.com:myblog.git
    Teams: team1, team2
    Units:
    +----------+---------+
    | Unit     | State   |
    +----------+---------+
    | myblog/0 | started |
    | myblog/1 | started |
    +----------+---------+

tsuru uses information from git configuration to guess the name of the app, for
more details, see `"Guessing app names"
<http://godoc.org/github.com/tsuru/tsuru-client/tsuru#hdr-Guessing_app_names>`_
section of tsuru command documentation.

Public Keys
===========

You can try to push now, but you'll get a permission error, because you haven't
pushed your key yet.

.. highlight:: bash

::

    $ tsuru key-add

This will search for a `id_rsa.pub` file in ~/.ssh/, if you don't have a
generated key yet, you should generate one before running this command.

If you have a public key in other format (for example, DSA), you can also give
the public key file to `key-add
<http://godoc.org/github.com/tsuru/tsuru-client/tsuru#hdr-Add_SSH_public_key_to_tsuru_s_git_server>`_:

.. highlight:: bash

::

    $ tsuru key-add $HOME/.ssh/id_dsa.pub

After your key is added, you can push your application to your cloud:

.. highlight:: bash

::

    $ git push tsuru master

Running commands
================

After that, you can check your app's url in the browser and see your app there.
You'll probably need to run migrations or other deploy related commands. To run
a single command, you should use the command `run
<http://godoc.org/github.com/tsuru/tsuru-client/tsuru#hdr-Run_an_arbitrary_command_in_the_app_machine>`_:

.. highlight:: bash

::

    $ tsuru run "python manage.py syncdb && python manage.py migrate"

Further instructions
====================

For a complete reference, check the documentation for tsuru command:
`<http://godoc.org/github.com/tsuru/tsuru-client/tsuru>`_.
