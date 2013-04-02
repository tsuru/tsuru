.. Copyright 2013 tsuru authors. All rights reserved.
   Use of this source code is governed by a BSD-style
   license that can be found in the LICENSE file.

++++++++++++++++++++++++++++++++++++++
Build your own PaaS with tsuru and lxc
++++++++++++++++++++++++++++++++++++++

This document describes how to create a private PaaS service using tsuru and lxc.

This document assumes that tsuru is being installed on a Ubuntu (12.10) machine. You
can use equivalent packages for beanstalkd, git, MongoDB and other tsuru
dependencies. Please make sure you satisfy minimal version requirements.

You can use the script bellow to quick install tsuru with lxc:

.. highlight:: bash

::

    $ curl https://raw.github.com/globocom/tsuru/master/misc/lxc-setup.bash | bash

Or follow this steps:

lxc
---


Install the lxc, by doing this:

.. highlight:: bash

::

    $ sudo apt-get install -y lxc

MongoDB
-------

Tsuru needs MongoDB stable, distributed by 10gen. `It's pretty easy to
get it running on Ubuntu <http://docs.mongodb.org/manual/tutorial/install-mongodb-on-ubuntu/>`_

Beanstalkd
----------

Tsuru uses `Beanstalkd <http://kr.github.com/beanstalkd/>`_ as a work queue.
Install the latest version, by doing this:

.. highlight:: bash

::

    $ sudo apt-get install -y beanstalkd

Configuring beanstalkd to start, edit the `/etc/default/beanstalkd` and uncomment this line:

::

    START=yes

Then start beanstalkd:

.. highlight:: bash

::

    $ sudo service beanstalkd start

Gandalf
-------

Tsuru uses `Gandalf <https://github.com/globocom/gandalf>`_ to manage git repositories, to get it installed `follow this steps <https://gandalf.readthedocs.org/en/latest/install.html>`_

Installing git
~~~~~~~~~~~~~~

.. highlight:: bash

::

    $ sudo apt-get install git -y

Creating git user
~~~~~~~~~~~~~~~~~

.. highlight:: bash

::

    $ sudo useradd git

Creating directories for repositories and template
~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~

Let's create the directory for bare repositories:

.. highlight:: bash

::

    $ sudo mkdir -p /var/repositories
    $ sudo chown -R git:git /var/repositories

And the directory for template and add the tsuru hooks:

.. highlight:: bash

::

    $ sudo mkdir -p /home/git/bare-template/hooks
    $ curl https://raw.github.com/globocom/tsuru/master/misc/git-hooks/post-receive > /home/git/bare-template/hooks/post-receive
    $ curl https://raw.github.com/globocom/tsuru/master/misc/git-hooks/pre-receive > /home/git/bare-template/hooks/pre-receive
    $ curl https://raw.github.com/globocom/tsuru/master/misc/git-hooks/pre-receivei.py > /home/git/bare-template/hooks/pre-receive.py
    $ sudo chown -R git:git /home/git/bare-template

Configuring gandalf
~~~~~~~~~~~~~~~~~~~

.. highlight:: bash

::

    sudo bash -c 'echo "bin-path: /usr/bin
    database:
      url: 127.0.0.1:27017
      name: gandalf
    git:
      bare:
        location: /var/repositories
        template: /home/git/bare-template
      daemon:
        export-all: true
    host: localhost
    webserver:
      port: \":8000\"" > /etc/gandalf.conf'

Nginx
-----

.. highlight:: bash

::

    $ sudo apt-get install -y nginx

Tsuru api and collector
-----------------------

You can download pre-built binaries of tsuru and collector. There are binaries
available only for Linux 64 bits, so make sure that ``uname -m`` prints
``x86_64``:

.. highlight:: bash

::

    $ uname -m
    x86_64

Then download and install the binaries. First, collector:

.. highlight:: bash

::

    $ curl -sL https://s3.amazonaws.com/tsuru/dist-server/tsuru-collector.tar.gz | sudo tar -xz -C /usr/bin

Then the API server:

.. highlight:: bash

::

    $ curl -sL https://s3.amazonaws.com/tsuru/dist-server/tsuru-api.tar.gz | sudo tar -xz -C /usr/bin

These commands will install ``collector`` and ``api`` commands in ``/usr/bin``
(you will need to be a sudoer and provide your password). You may install these
commands somewhere else in your ``PATH``.

Configuring
~~~~~~~~~~~

Before running tsuru, you must configure it. By default, tsuru will look for
the configuration file in the ``/etc/tsuru/tsuru.conf`` path. You can check a
sample configuration file and documentation for each tsuru setting in the
:doc:`"Configuring tsuru" </config>` page.

You can download the sample configuration file from Github:

.. highlight:: bash

::

    $ [sudo] mkdir /etc/tsuru
    $ [sudo] curl -sL https://raw.github.com/globocom/tsuru/master/etc/tsuru-lxc.conf -o /etc/tsuru/tsuru.conf

Downloading charms
~~~~~~~~~~~~~~~~~~

.. highlight:: bash

::

    $ git clone git://github.com/globocom/charms.git -b lxc /home/ubuntu/charms


Running
~~~~~~~

Now that you have ``api`` and ``collector`` properly installed, and you
:doc:`configured tsuru </config>`, you're three steps away from running it.


Start tsuru and collector

.. highlight:: bash

::

    $ collector &
    $ sudo api &

You can see the logs in:

.. highlight:: bash

::

    $ tail -f /var/log/syslog

Using tsuru
===========

Congratulations! At this point you should have a working tsuru server running
on your machine, follow the :doc:`tsuru client usage guide
</apps/client/usage>` to start build your apps.
