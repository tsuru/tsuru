.. Copyright 2016 tsuru authors. All rights reserved.
   Use of this source code is governed by a BSD-style
   license that can be found in the LICENSE file.

.. meta::
    :description: Install tsuru-admin client
    :keywords: paas, cloud computing, tsuru

++++++++++++++++++++++
Installing tsuru-admin
++++++++++++++++++++++

`tsuru-admin` is a client with commands that will help you to manage a tsuru
cluster.

This document describes how you can install `tsuru-admin`, using pre-compiled
binaries, packages or building them from source.

- `Downloading binaries (Mac OS X, Linux and Windows)`_
- `Using homebrew (Mac OS X only)`_
- `Using the PPA (Ubuntu only)`_
- `Build from source (Linux, Mac OS X and Windows)`_

Downloading binaries (Mac OS X, Linux and Windows)
==================================================

We provide pre-built binaries for OS X and Linux, only for the amd64
architecture. You can download these binaries directly from the releases page
of the project:

    * tsuru-admin: https://github.com/tsuru/tsuru-admin/releases

Using homebrew (Mac OS X only)
==============================

If you use Mac OS X and `homebrew <http://mxcl.github.com/homebrew/>`_, you may
use a custom tap to install ``tsuru-admin``. First you
need to add the tap:

.. highlight:: bash

::

    $ brew tap tsuru/homebrew-tsuru

Now you can install tsuru-admin:

.. highlight:: bash

::

    $ brew install tsuru-admin

Whenever a new version of any of tsuru's clients is out, you can just run:

.. highlight:: bash

::

    $ brew update
    $ brew upgrade tsuru-admin

For more details on taps, check `homebrew documentation
<https://github.com/Homebrew/homebrew/wiki/brew-tap>`_.

**NOTE:** tsuru clients require Go 1.4 or higher. Make sure you have the last version
of Go installed in your system.

Using the PPA (Ubuntu only)
===========================

Ubuntu users can install tsuru clients using ``apt-get`` and the `tsuru PPA
<https://launchpad.net/~tsuru/+archive/ppa>`_. You'll need to add the PPA
repository locally and run an ``apt-get update``:

.. highlight:: bash

::

    $ sudo apt-add-repository ppa:tsuru/ppa
    $ sudo apt-get update

Now you can install tsuru-admin:

.. highlight:: bash

::

    $ sudo apt-get install tsuru-admin

Build from source (Linux, Mac OS X and Windows)
===============================================

.. note::

    If you're feeling adventurous, you can try it on other platforms, like
    FreeBSD and OpenBSD. Please let us know about your progress!

`tsuru's source <https://github.com/tsuru/tsuru>`_ is written in `Go
<http://golang.org>`_, so before installing tsuru from source, please make sure
you have `installed and configured Go <http://golang.org/doc/install>`_.

With Go installed and configured, you can use ``go get`` to install any of
tsuru's clients:

.. highlight:: bash

::

    $ go get github.com/tsuru/tsuru-admin
