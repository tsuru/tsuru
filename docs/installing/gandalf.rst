.. Copyright 2015 tsuru authors. All rights reserved.
   Use of this source code is governed by a BSD-style
   license that can be found in the LICENSE file.

+++++++
Gandalf
+++++++

tsuru optionally uses gandalf to manage Git repositories used to push
applications to. It's also responsible for setting hooks in these repositories
which will notify the tsuru API when a new deploy is made. For more details
check `Gandalf Documentation <http://gandalf.readthedocs.org/>`_

This document will focus on how to setup a Gandalf installation with the necessary
hooks to notify the tsuru API.

Adding repositories
===================

Let's start adding the repositories for tsuru which contain the Gandalf package.

.. highlight:: bash

::

    sudo apt-get update
    sudo apt-get install curl python-software-properties
    sudo apt-add-repository ppa:tsuru/ppa -y
    sudo apt-get update

Installing
==========

.. highlight:: bash

::

    sudo apt-get install gandalf-server

A deploy is executed in the ``git push``. In order to get it working, you will
need to add a pre-receive hook. tsuru comes with three pre-receive hooks, all
of them need further configuration:

    * s3cmd: uses `Amazon S3 <https://s3.amazonaws.com>`_ to store and serve
      archives
    * archive-server: uses tsuru's `archive-server
      <https://github.com/tsuru/archive-server>`_ to store and serve archives
    * swift: uses `Swift <http://swift.openstack.org>`_ to store and server
      archives (compatible with `Rackspace Cloud Files
      <http://www.rackspace.com/cloud/files/>`_)

In this documentation, we will use archive-server, but you can use anything that
can store a git archive and serve it via HTTP or FTP. You can install archive-
server via apt-get too:

.. highlight:: bash

::

    sudo apt-get install archive-server

Then you will need to configure Gandalf, install the pre-receive hook, set the
proper environment variables and start Gandalf and the archive-server, please note
that you should replace the value ``<your-machine-addr>`` with your machine public
address:

.. highlight:: bash

::

    sudo mkdir -p /home/git/bare-template/hooks
    sudo curl https://raw.githubusercontent.com/tsuru/tsuru/master/misc/git-hooks/pre-receive.archive-server -o /home/git/bare-template/hooks/pre-receive
    sudo chmod +x /home/git/bare-template/hooks/pre-receive
    sudo chown -R git:git /home/git/bare-template
    cat | sudo tee -a /home/git/.bash_profile <<EOF
    export ARCHIVE_SERVER_READ=http://<your-machine-addr>:3232 ARCHIVE_SERVER_WRITE=http://127.0.0.1:3131
    EOF

In the ``/etc/gandalf.conf`` file, remove the comment from the line "template:
/home/git/bare-template", so it looks like that:

.. highlight:: yaml

::

    git:
      bare:
        location: /var/lib/gandalf/repositories
        template: /home/git/bare-template

Then start gandalf and archive-server:

.. highlight:: bash

::

    sudo start gandalf-server
    sudo start archive-server

Configuring tsuru to use Gandalf
================================

In order to use Gandalf, you need to change tsuru.conf accordingly:

#. Define "repo-manager" to use "gandalf";
#. Define "git:api-server" to point to the API of the Gandalf server
   (example: "http://localhost:8000");
#. Define "git:unit-repo" to point to the directory where code will live in the
   application unit (example: "/home/application/current").

For more details, please refer to the :doc:`configuration page
</reference/config>`.

Token for authentication with tsuru API
=======================================

There is one last step in configuring Gandalf. It involves generating an access
token so that the hook we created can access the tsuru API. This must be done
after installing the tsuru API and it's detailed in the next :ref:`installation
step <gandalf_auth_token>`.
