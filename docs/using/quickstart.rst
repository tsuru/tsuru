.. Copyright 2015 tsuru authors. All rights reserved.
   Use of this source code is governed by a BSD-style
   license that can be found in the LICENSE file.

++++++++++++++++++++++++++
Building your app in tsuru
++++++++++++++++++++++++++

tsuru is an open source polyglot cloud application platform. With tsuru, you
don't need to think about servers at all. You:

- Write apps in the programming language of your choice
- Back it with add-on resources (tsuru calls these *services*) such as SQL and
  NoSQL databases, memcached, redis, and many others.
- Manage your app using the ``tsuru`` command-line tool
- Deploy code using the Git revision control system

tsuru takes care of where in your cluster to run your apps and the services
they use. You can then focus on making your apps awesome.


Install the tsuru client
++++++++++++++++++++++++

:doc:`Install the tsuru client </using/install-client>` for your development platform.

The ``tsuru`` client is a command-line tool for creating and managing apps.
Check out the :doc:`CLI usage guide </reference/tsuru-client>` to learn more.

Sign up
+++++++

To create an account, you use the command `user-create`:

.. highlight:: bash

::

    $ tsuru user-create youremail@domain.com

``user-create`` will ask for the desired password twice.

Login
+++++

To login in tsuru, you use the command `login`:

.. highlight:: bash

::

    $ tsuru login youremail@domain.com

It will ask for your password. Unless your tsuru installation is configured to
use OAuth.


Deploy an application
+++++++++++++++++++++

Choose from the following getting started tutorials to learn how to deploy your
first application using one of the supported platforms:

* :doc:`Deploying Python applications in tsuru </using/python>`
* :doc:`Deploying Ruby/Rails applications in tsuru </using/ruby>`
* :doc:`Deploying PHP applications in tsuru </using/php>`
* :doc:`Deploying go applications in tsuru </using/go>`
