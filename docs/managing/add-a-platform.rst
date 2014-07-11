.. Copyright 2014 tsuru authors. All rights reserved.
   Use of this source code is governed by a BSD-style
   license that can be found in the LICENSE file.

++++++++++++++++++++++++++++
Creating a platform to tsuru
++++++++++++++++++++++++++++

Overview
========

Tsuru allows you create apps the way you want using the platform you want.
But for that you need to be create a platform prepared for run with.

To Tsuru be able to use your platform you need to configure the following scripts on **/var/lib/tsuru**:

    * deploy
    * restart
    * start

Tsuru has a `base platform <https://github.com/tsuru/basebuilder>`_ that you can use to base yours.

Using docker
============

Now we will create a whole new platform `with docker <http://www.docker.com/>`_, `circus <https://circus.readthedocs.org/en/>`_ and tsuru basebuilder.
Tsuru basebuilder provides to us some useful scripts like **install, setup and start**.

So, using the base platform provided by tsuru we can write a Dockerfile like that:

.. highlight:: bash

::

    from	ubuntu:14.04
    run	apt-get install wget -y --force-yes
    run	wget http://github.com/tsuru/basebuilder/tarball/master -O basebuilder.tar.gz --no-check-certificate
    run	mkdir /var/lib/tsuru
    run	tar -xvf basebuilder.tar.gz -C /var/lib/tsuru --strip 1
    run	cp /var/lib/tsuru/base/restart /var/lib/tsuru
    run	cp /var/lib/tsuru/base/start /var/lib/tsuru
    run cp /home/your-user/deploy /var/lib/tsuru
    run	/var/lib/tsuru/base/install
    run	/var/lib/tsuru/base/setup

Adding your platform to tsuru
=============================

If you create a platform using docker, you can use the tsuru-admin cmd to add that.

.. highlight:: bash

::

    $ tsuru-admin platform-add your-platform-name --dockerfile http://url-to-dockerfile
