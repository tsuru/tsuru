.. Copyright 2013 tsuru authors. All rights reserved.
   Use of this source code is governed by a BSD-style
   license that can be found in the LICENSE file.

++++++++++++++++++++++++++
Building your app in tsuru
++++++++++++++++++++++++++

Tsuru is an open source polyglot cloud application platform. With Tsuru, you
don't need to think about servers at all. You can write apps in the programming
language of your choice, back it with add-on resources such as SQL and NoSQL
databases, memcached, redis, and many others. You manage your app using the
Tsuru command-line tool and you deploy code using the Git revision control
system, all running on the Tsuru infrastructure.


Install the tsuru client
++++++++++++++++++++++++

:doc:`Install the Tsuru client </install/client>` for your development platform.

The the Tsuru client is a command-line tool for creating and managing apps.
Check out the :doc:`CLI usage guide </apps/client/usage>` to learn more.

Sign up
+++++++

To create an account, you use the `user-create
<http://godoc.org/github.com/globocom/tsuru/cmd/tsuru#hdr-Create_a_user>`_
command:

.. highlight:: bash

::

    $ tsuru user-create youremail@domain.com

``user-create`` will ask for your password twice.

Login
+++++

To login in tsuru, you use the `login
<http://godoc.org/github.com/globocom/tsuru/cmd/tsuru#hdr-Authenticate_within_remote_tsuru_server>`_
command, you will be asked for your password:

.. highlight:: bash

::

    $ tsuru login youremail@domain.com


Deploy an application
+++++++++++++++++++++

Choose from the following getting started tutorials to learn how to deploy your
first application using a supported language or framework:

* :doc:`Deploying Python applications in tsuru </apps/quickstart/python>`
* :doc:`Deploying Ruby/Rails applications in tsuru </apps/quickstart/ruby>`
* :doc:`Deploying PHP applications in tsuru </apps/quickstart/php>`
