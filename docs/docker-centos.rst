.. Copyright 2013 tsuru authors. All rights reserved.
   Use of this source code is governed by a BSD-style
   license that can be found in the LICENSE file.

+++++++++++++++++++++++++++++++++++++++++++++++++++
Build your own PaaS with tsuru and Docker on Centos
+++++++++++++++++++++++++++++++++++++++++++++++++++

This document describes how to create a private PaaS service using tsuru and docker on Centos.

This document assumes that tsuru is being installed on a Centos (6.4+) machine. You
can use equivalent packages for beanstalkd, git, MongoDB and other tsuru
dependencies. Please make sure you satisfy minimal version requirements.

Just follow this steps:

Docker
------

To make docker working on a RHEL/Centos distro, you will need to use the `EPEL repository <http://fedoraproject.org/wiki/EPEL>`_, build a kernel with `AUFS <http://aufs.sourceforge.net/>`_ support, and install all dependencies as following: 

.. highlight:: bash

::

    # Installing the EPEL respository
    $ rpm -iUvh http://dl.fedoraproject.org/pub/epel/6/x86_64/epel-release-6-8.noarch.rpm
    $ yum update -y
    # Download the kernel + dependencies for docker 
    $ yum install fedora-packager -y
    # you will need to perform these steps bellow with a unprivileged user, ex: su - tsuru
    $ git clone https://github.com/sciurus/docker-rhel-rpm
    $ cd docker-rhel-rpm
    # Fix the AUFSver with the latest AUFS for kernel 3.10 version
    # To get the latest id just execute this command: 
    # curl -s http://sourceforge.net/p/aufs/aufs3-standalone/ci/aufs3.10/tree/ | awk -F/ '/Tree/{print $6}'
    $ sed -i 's/\(^.*AUFSver aufs-aufs3-standalone\).*$/\1-38c1b30224c440e3618c90633bef73cdce54c6fd/' kernel-ml-aufs/kernel-ml-aufs-3.10.spec
    # Remove auto restart of docker, as it will be managed by circus
    $ sed -i 's|^%{_sysconfdir}/init/docker.conf||; s/.*source1.*//i' docker/docker.spec

Now, just follow the steps to build the kernel + lxc + docker from `here: https://github.com/sciurus/docker-rhel-rpm/blob/master/README.md <https://github.com/sciurus/docker-rhel-rpm/blob/master/README.md>`_

.. highlight:: bash

::

    # In order to use docker, you will need to allow the ip forward
    $ grep ^net.ipv4.ip_forward /etc/sysctl.conf > /dev/null 2>&1 && \
                        sed -i 's/^net.ipv4.ip_forward.*/net.ipv4.ip_forward = 1/' /etc/sysctl.conf  || \
                        echo 'net.ipv4.ip_forward = 1' >> /etc/sysctl.conf
    $ sysctl -p
    # You also need to disable selinux, adding the parameter "selinux=0" in your new kernel 3.10 (/boot/grub/grub.conf)
    $ grep selinux=0 /boot/grub/menu.lst
    # Turn off your default firewall rules for now
    $ service iptables stop
    $ chkconfig iptables off


After build, install and reboot the server with the new kernel(it will take some time), you will need to install the tsuru's dependencies 


Tsuru's Dependencies
--------------------

Tsuru needs MongoDB stable, distributed by 10genr, `Beanstalkd <http://kr.github.com/beanstalkd/>`_ as work queue, git-daemon(necessary for Gandalf) and Redis for `hipache <https://github.com/dotcloud/hipache/>`_ pt-ge
Install the latest EPEL version, by doing this:

.. highlight:: bash

::

    $ yum install mongodb-server beanstalkd git-daemon redis python-pip python-devel gcc gcc-c++ -y 
    $ service mongod start
    $ service beanstalkd start
    $ service redis start
    $ chkconfig mongod on
    $ chkconfig beanstalkd on
    $ chkconfig redis on


Tsuru Setup
-----------

Tsuru uses `Gandalf <https://github.com/globocom/gandalf/>`_ to manage `git repositories <https://gandalf.readthedocs.org/en/latest/install.html/>`_, and `hipache <https://github.com/dotcloud/hipache/>`_ as router
To setup Tsuru, just follow this steps. Obs: It can be used to upgrade this services as needed

.. highlight:: bash

::

    $ curl https://raw.github.com/globocom/tsuru/master/misc/functions-docker-centos.sh -o functions-docker-centos.sh
    $ source functions-docker-centos.sh
    # Install Tsuru Server(tsr), Gandalf, Hipache and Circus for monitoring
    $ install_services


Configuring
~~~~~~~~~~~

Before running tsuru, you must configure it. By default, tsuru will look for
the configuration file in the ``/etc/tsuru/tsuru.conf`` path. You can check a
sample configuration file and documentation for each tsuru setting in the
:doc:`"Configuring tsuru" </config>` page.

You can download the sample configuration file from `Github <https://raw.github.com/globocom/tsuru/master/etc/tsuru-docker.conf/>`_ 

By default, this configuration will use the tsuru image namespace, so if you try to create an application using python platform,
tsuru will search for an image named tsuru/python. You can change this default behavior by changing the docker:repository-namespace config field.

To automatically configure tsuru and all other services, just run the function presented in functions-docker-centos.sh file, as following

.. highlight:: bash

::

    # It will configure tsuru, gandalf, hipache and circus. If you had already done that before, your previously configuration will be lost
    $ source functions-docker-centos.sh #you already did it above
    $ configure_services_for_first_time
    # start circus
    $ initctl start circusd

At that time, circus should be running and started all the tsuru services

Running
~~~~~~~

Now that you have ``tsr`` properly installed, and you
:doc:`configured tsuru </config>`
Verify api, collector and docker-ssh-agent

.. highlight:: bash

::

    $ ps -ef|grep ts[r]

Creating Docker Images
~~~~~~~~~~~~~~~~~~~~~~

Now it's time to install the docker images for your neededs platform. You can build your own docker image, or you can use ours own images as following

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
    # Now try to create your apps!

Using tsuru
===========

Congratulations! At this point you should have a working tsuru server running
on your machine, follow the :doc:`tsuru client usage guide
</apps/client/usage>` to start build your apps.


If you want to add services - and see all the power of tsuru(like the bind command) - just use `crane <http://docs.tsuru.io/en/latest/services/usage.html>`_ 
