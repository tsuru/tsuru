.. Copyright 2017 tsuru authors. All rights reserved.
   Use of this source code is governed by a BSD-style
   license that can be found in the LICENSE file.

++++++++++
App-Deploy
++++++++++

Overview
++++++++

This is a hands on guide to deploy a simple app using tsuru's CLI ``app deploy`` command.

Creating a app
++++++++++++++

To create an app, you need to use the command `app create`:

.. highlight:: bash

::

    $ tsuru app create <app-name> <app-platform> <team>

Deploying an app
++++++++++++++++

To deploy your first app after choosing your ``<app-name>`` and ``<app-platform>``, we can deploy It using this template:

.. highlight:: bash

::

    $ tsuru app deploy -a <app-name> <directory>

As an example we can deploy a tutorial app named ``hello world``:

.. highlight:: bash

::

    $ tsuru app deploy -a helloworld .

With the command below we'll be able to deploy our first app ``helloworld`` that is situated on the current directory (``"."``).

Ignoring files and directories
++++++++++++++++++++++++++++++

To deploy smaller applications you are allowed to ignore files and/or directories using a file named ``.tsuruignore`` that needs to be on your app's root directory. After using `app deploy`, ``.tsuruignore`` will be read and each line will be considered a pattern to be ignored, so anything that matches a pattern will not be on your app after the deployment.

This is not mandatory while deploying your app, so If there's no ``.tsuruignore`` on your app root directory, It'll deploy your normally. This is a example of a ``.tsuruignore`` file:

.. highlight:: go

::

    <file name>.<file type>     // e.g.: app.py
    *.py                        // any named file of this type of file
    app.*                       // any type of file with this name
    directory
    dir*ry                      // anything that matches these pieces of name
    dir/to/specific/path/<file name>.<file type>
    relative/dir/*/to/path      // any directory that leads to <path>
