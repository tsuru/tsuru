.. Copyright 2014 tsuru authors. All rights reserved.
   Use of this source code is governed by a BSD-style
   license that can be found in the LICENSE file.

++++++++++++++++++++++++++
Building your app in tsuru
++++++++++++++++++++++++++

tsuru is an open source polyglot cloud application platform. With tsuru, you
don't need to think about servers at all. You:

- Write apps in the programming language of your choice
- Back it with add-on resources (tsuru calls these *services*) such as SQL and NoSQL databases, memcached, redis, and many others.
- Manage your app using the ``tsuru`` command-line tool
- Deploy code using the Git revision control system

tsuru takes care of where in your cluster to run your apps and the services they use. You can focus on making your apps awesome. 


Install the tsuru client
++++++++++++++++++++++++

:doc:`Install the tsuru client </using/install-client>` for your development platform.

The ``tsuru`` client is a command-line tool for creating and managing apps.
Check out the :doc:`CLI usage guide </reference/tsuru-client>` to learn more.

Sign up
+++++++

To create an account, you use the `user-create
<http://godoc.org/github.com/tsuru/tsuru-client/tsuru#hdr-Create_a_user>`_
command:

.. highlight:: bash

::

    $ tsuru user-create youremail@domain.com

``user-create`` will ask for your password twice.

Login
+++++

To login in tsuru, you use the `login
<http://godoc.org/github.com/tsuru/tsuru-client/tsuru#hdr-Authenticate_within_remote_tsuru_server>`_
command, you will be asked for your password:

.. highlight:: bash

::

    $ tsuru login youremail@domain.com


Deploy an application
+++++++++++++++++++++

Choose from the following getting started tutorials to learn how to deploy your
first application using a supported language or framework:

* :doc:`Deploying Python applications in tsuru </using/python>`
* :doc:`Deploying Ruby/Rails applications in tsuru </using/ruby>`
* :doc:`Deploying PHP applications in tsuru </using/php>`
* :doc:`Deploying go applications in tsuru </using/go>`
