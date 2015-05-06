.. Copyright 2015 tsuru authors. All rights reserved.
   Use of this source code is governed by a BSD-style
   license that can be found in the LICENSE file.


++++++++++++++++++++++++++++
Creating a platform
++++++++++++++++++++++++++++

Overview
========

If you need a platform that's not already available in our `platforms repository
<https://github.com/tsuru/basebuilder>`_ it's pretty easy to create a new one
based on a existing one.

To tsuru to be able to use your platform you only need to have the following
scripts available on **/var/lib/tsuru**:

    * /var/lib/tsuru/deploy
    * /var/lib/tsuru/start


Using Docker
============

Now we will create a whole new platform `with Docker <http://www.docker.com/>`_,
`circus <https://circus.readthedocs.org/en/>`_ and tsuru basebuilder. tsuru
basebuilder provides to us some useful scripts like **install, setup and start**.

So, using the base platform provided by tsuru we can write a Dockerfile like that:

.. highlight:: bash

::

    from ubuntu:14.04
    run  apt-get install wget -y --force-yes
    run  wget http://github.com/tsuru/basebuilder/tarball/master -O basebuilder.tar.gz --no-check-certificate
    run  mkdir /var/lib/tsuru
    run  tar -xvf basebuilder.tar.gz -C /var/lib/tsuru --strip 1
    run  cp /var/lib/tsuru/base/start /var/lib/tsuru
    run  cp /home/your-user/deploy /var/lib/tsuru
    run  /var/lib/tsuru/base/install
    run  /var/lib/tsuru/base/setup

Adding your platform to tsuru
=============================

After creating you platform as a Docker image, you can add it to tsuru using
``tsuru-admin``:

.. highlight:: bash

::

    $ tsuru-admin platform-add your-platform-name --dockerfile http://url-to-dockerfile
