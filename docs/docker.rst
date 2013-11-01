.. Copyright 2013 tsuru authors. All rights reserved.
   Use of this source code is governed by a BSD-style
   license that can be found in the LICENSE file.

+++++++++++++++++++++++++++++++++++++++++
Build your own PaaS with tsuru and Docker
+++++++++++++++++++++++++++++++++++++++++

This document describes how to create a private PaaS service using tsuru and docker.

This document assumes that tsuru is being installed on a Ubuntu 13.04 64-bit
machine. If you want to use Ubuntu LTS vesion see `docker documentation
<http://docs.docker.io/en/latest/installation/ubuntulinux/#ubuntu-precise-12-04-lts-64-bit>`_
on how to install it.  You can use equivalent packages for beanstalkd, git,
MongoDB and other tsuru dependencies. Please make sure you satisfy minimal
version requirements.

You can use the scripts bellow to quick install tsuru with docker:

.. highlight:: bash

::

    $ curl -O https://raw.github.com/globocom/tsuru/master/misc/docker-setup.bash; bash docker-setup.bash

Or follow this steps:

DNS server
----------

You can integrate any DNS server with tsuru. Here:
`<http://docs.tsuru.io/en/latest/misc/dns-forwarders.html>`_ you can find a
example of how to install a DNS server integrated with tsuru

docker
------


Install docker:

.. highlight:: bash

::

    $ wget -qO- https://get.docker.io/gpg | sudo apt-key add -
    $ echo 'deb http://get.docker.io/ubuntu docker main' | sudo tee /etc/apt/sources.list.d/docker.list
    $ sudo apt-get update
    $ sudo apt-get install lxc-docker

Then edit ``/etc/init/docker.conf`` to start docker on tcp://127.0.0.1:4243:

.. highlight:: bash

::

    $ cat > /etc/init/docker.conf <<EOF
    description     "Docker daemon"

    start on filesystem and started lxc-net
    stop on runlevel [!2345]

    respawn

    script
        /usr/bin/docker -H tcp://127.0.0.1:4243 -d
    end script

    EOF

MongoDB
-------

Tsuru needs MongoDB stable, distributed by 10gen. `It's pretty easy to get it
running on Ubuntu
<http://docs.mongodb.org/manual/tutorial/install-mongodb-on-ubuntu/>`_

Redis
-----

Tsuru uses Redis to communicate new routes. By default it points to a locally installed Redis server. 
Install on Ubuntu via `apt-get`:

::

	$ sudo apt-get install redis-server

If you will use a remote Redis server, skip this and point your server on `/etc/tsuru/tsuru.conf`

Beanstalkd
----------

Tsuru uses `Beanstalkd <http://kr.github.com/beanstalkd/>`_ as a work queue.
Install the latest version, by doing this:

.. highlight:: bash

::

    $ sudo apt-get install beanstalkd

Configuring beanstalkd to start, edit the `/etc/default/beanstalkd` and
uncomment this line:

::

    START=yes

Then start beanstalkd:

.. highlight:: bash

::

    $ sudo service beanstalkd start

Gandalf
-------

Tsuru uses `Gandalf <https://github.com/globocom/gandalf>`_ to manage Git
repositories, you can install it from `Tsuru PPA
<https://launchpad.net/~tsuru/+archive/ppa>`_:

.. highlight:: bash

::

    $ sudo apt-add-repository ppa:tsuru/ppa
    $ sudo apt-get update
    $ sudo apt-get install gandalf-server

Creating directory for bare template
~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~

Tsuru depends on some Git hooks, you will need to create the bare template
directory, and download the hook from Tsuru repository:

.. highlight:: bash

::

    $ sudo mkdir -p /home/git/bare-template/hooks
    $ curl https://raw.github.com/globocom/tsuru/master/misc/git-hooks/post-receive | sudo tee /home/git/bare-template/hooks/post-receive
    $ sudo chown -R git:git /home/git/bare-template

Configuring gandalf
~~~~~~~~~~~~~~~~~~~

.. highlight:: bash

::

    $ cat > /etc/gandalf.conf <<EOF
    bin-path: /usr/bin/gandalf-ssh
    database:
      url: 127.0.0.1:27017
      name: gandalf
    git:
      bare:
        location: /var/lib/gandalf/repositories
        template: /home/git/bare-template
    host: localhost
    bind: 127.0.0.1:8000
    EOF

Change the ``host: localhost`` to your base domain.

Starting Gandalf and git-daemon
~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~

This one is easy:

.. highlight:: bash

::

    $ sudo start git-daemon
    $ sudo start gandalf-server

Tsuru API and collector
-----------------------

You can also install Tsuru API and Collector from Tsuru PPA:

.. highlight:: bash

::

    $ sudo apt-get install tsuru-server gandalf-server

Configuring
~~~~~~~~~~~

Before running tsuru, you must configure it. By default, tsuru will look for
the configuration file in the ``/etc/tsuru/tsuru.conf`` path. You can check a
sample configuration file and documentation for each tsuru setting in the
:doc:`"Configuring tsuru" </config>` page.

The debian package will create the file, you may open it and customize some
settings, or you can download the sample configuration file from Github:

.. highlight:: bash

::

    $ sudo curl -sL https://raw.github.com/globocom/tsuru/master/etc/tsuru-docker.conf -o /etc/tsuru/tsuru.conf

By default, this configuration will use the tsuru image namespace, so if you
try to create an application using python platform, tsuru will search for an
image named tsuru/python. You can change this default behavior by changing the
docker:repository-namespace config field.

You'll also need to enable Tsuru API, Collector and SSH agent on
``/etc/default/tsuru-server``:

.. highlight:: bash

::

    $ cat > /etc/default/tsuru-server <<EOF
    TSR_API_ENABLED=yes
    TSR_COLLECTOR_ENABLED=yes

    TSR_SSH_AGENT_ENABLED=yes
    TSR_SSH_AGENT_USER=ubuntu
    TSR_SSH_AGENT_LISTEN=127.0.0.1:4545
    TSR_SSH_AGENT_PRIVATE_KEY=/var/lib/tsuru/.ssh/id_rsa
    EOF

Running
~~~~~~~

Now that you have ``tsr`` properly installed, and you :doc:`configured tsuru
</config>`, you're three steps away from running it.

Start api, collector and docker-ssh-agent

.. highlight:: bash

::

    $ sudo start tsuru-server-collector
    $ sudo start tsuru-server-api
    $ sudo start tsuru-ssh-agent

You can see the logs in:

.. highlight:: bash

::

    $ sudo tail -f /var/log/syslog


Creating Docker Images
======================

Now it's time to import the Docker images for your platforms. You can build
your own docker image, or you can use our images as following:

.. highlight:: bash

::

    # Add an alias for docker to make your life easier (add it to your .bash_profile)
    $ alias docker='docker -H 127.0.0.1:4243'
    # Build the wanted platform, here we are adding the static platform(webserver)
    $ docker build -t tsuru/static https://raw.github.com/flaviamissi/basebuilder/master/static/Dockerfile
    # Now you can see if your image is ready - you should see the tsuru/static as an repository
    $ docker images
    # If you want all the other platforms, just run the command bellow
    $ for image in nodejs php python ruby; do docker build -t tsuru/$image https://raw.github.com/flaviamissi/basebuilder/master/$image/Dockerfile;done
    # To see if everything went well - just take a look in the repository column
    $ docker images
    # Now create your apps!

Using tsuru
===========

Congratulations! At this point you should have a working tsuru server running
on your machine, follow the :doc:`tsuru client usage guide
</apps/client/usage>` to start build your apps.

Adding Services
===============

Here you will find a complete step-by-step example of how to install a mysql
service with tsuru: `http://docs.tsuru.io/en/latest/services/mysql-example.html
<http://docs.tsuru.io/en/latest/services/mysql-example.html>`_
