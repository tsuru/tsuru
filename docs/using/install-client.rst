.. Copyright 2013 tsuru authors. All rights reserved.
   Use of this source code is governed by a BSD-style
   license that can be found in the LICENSE file.

.. meta::
    :description: Install guide for tsuru client
    :keywords: paas, cloud computing, tsuru

+++++++++++++++++++++++
Installing tsuru client
+++++++++++++++++++++++

**tsuru** is the command line utility used by application developers, that
will allow users to create, list, bind and manage apps. For more details,
check :doc:`tsuru usage </reference/tsuru-client>`.

This document describes how you can install tsuru CLI, using pre-compiled
binaries, packages or building them from source.

- `Easy install (Mac OS X and Linux)`_
- `Downloading binaries (Mac OS X, Linux and Windows)`_
- `Using homebrew (Mac OS X only)`_
- `Using the packagecloud.io (Linux)`_
- `Build from source (Linux, Mac OS X and Windows)`_

Easy install (Mac OS X and Linux)
==================================================

Use a script to download and install the latest version of tsuru client for you.

.. highlight:: bash

::

    $ curl -fsSL "https://tsuru.io/get" | bash


Downloading binaries (Mac OS X, Linux and Windows)
==================================================

We provide pre-built binaries for OS X and Linux, only for the amd64
architecture. You can download these binaries directly from the releases page
of the project:

    * tsuru: https://github.com/tsuru/tsuru-client/releases

Using homebrew (Mac OS X only)
==============================

If you use Mac OS X and `homebrew <http://mxcl.github.com/homebrew/>`_, you may
use a custom tap to install ``tsuru``. First you need to add the tap:

.. highlight:: bash

::

    $ brew tap tsuru/homebrew-tsuru

Now you can install tsuru:

.. highlight:: bash

::

    $ brew install tsuru

Whenever a new version of any of tsuru clients is out, you can just run:

.. highlight:: bash

::

    $ brew update
    $ brew upgrade tsuru

For more details on taps, check `homebrew documentation
<https://github.com/Homebrew/homebrew/wiki/brew-tap>`_.

**NOTE:** tsuru client require Go 1.4 or higher. Make sure you have the last version
of Go installed in your system.

Using the packagecloud.io (Linux)
=================================

**Quick install**

deb:

.. highlight:: bash

::

    $ curl -s https://packagecloud.io/install/repositories/tsuru/stable/script.deb.sh | sudo bash
    $ sudo apt-get install tsuru-client

rpm:

.. highlight:: bash

::

    $ curl -s https://packagecloud.io/install/repositories/tsuru/stable/script.rpm.sh | sudo bash
    $ sudo yum install tsuru-client

For more details, check `packagecloud.io documentation
<https://packagecloud.io/tsuru/stable/install#bash>`_.

Build from source (Linux, Mac OS X and Windows)
===============================================

.. note::

    If you're feeling adventurous, you can try it on other platforms, like
    FreeBSD and OpenBSD. Please let us know about your progress!

`tsuru's source <https://github.com/tsuru/tsuru>`_ is written in `Go
<http://golang.org>`_, so before installing tsuru from source, please make sure
you have `installed and configured Go <http://golang.org/doc/install>`_.

With Go installed and configured, you can use ``go get`` to install tsuru
client:

.. highlight:: bash

::

    $ go get github.com/tsuru/tsuru-client/tsuru
