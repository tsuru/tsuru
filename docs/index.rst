.. Copyright 2013 tsuru authors. All rights reserved.
   Use of this source code is governed by a BSD-style
   license that can be found in the LICENSE file.

+++++++++++++
Documentation
+++++++++++++

What is Tsuru?
--------------

Tsuru is an open source polyglot cloud application platform (paas). With tsuru, you don’t need to think about servers at all. You can write apps in the programming language of your choice, back it with add-on resources such as SQL and NoSQL databases, memcached, redis, and many others. You manage your app using the tsuru command-line tool and you deploy code using the Git revision control system, all running on the tsuru infrastructure.

Learn more in :doc:`Tsuru's Overview </overview>` or check out our :doc:`FAQ </faq>`.

* :doc:`Build your own PaaS with Tsuru </build>`
* :doc:`Deploy your application on Tsuru </apps/quickstart>`
    * :doc:`python/django </apps/quickstart/python>`
    * :doc:`ruby/rails </apps/quickstart/ruby>`
    * :doc:`php </apps/quickstart/php>`
* :doc:`Provide services on Tsuru </services/build>`

More documentation
------------------

For tsuru users
+++++++++++++++

* :doc:`clients installation guide </install/client>`
* :doc:`tsuru client usage guide </apps/client/usage>`
* :doc:`using services </apps/client/services>`
* :doc:`building your application </apps/quickstart>`
    * :doc:`python/django </apps/quickstart/python>`
    * :doc:`ruby/rails </apps/quickstart/ruby>`
    * :doc:`php </apps/quickstart/php>`

For tsuru ops
+++++++++++++

* :doc:`build your own PaaS using juju </build>`
* :doc:`build your own PaaS using docker </docker>`
* :doc:`tsuru configuration </config>`
* :doc:`backing up tsuru </server/backup>`
* :doc:`tsuru api reference </api>`

* :doc:`building your service tutorial </services/build>`
* :doc:`crane usage guide </services/usage>`
* :doc:`tsuru services api workflow </services/api>`


Contributions and Feedback
++++++++++++++++++++++++++

* :doc:`how to contribute </contribute>`
* :doc:`coding style </contribute/coding-style>`
* :doc:`setting up your tsuru development environment </contribute/setting-up-your-tsuru-development-environment>`
* :doc:`community </community>`

.. toctree::
    :hidden:
    :glob:

    *
    */*
    */*/*
