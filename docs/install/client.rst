.. Copyright 2013 tsuru authors. All rights reserved.
   Use of this source code is governed by a BSD-style
   license that can be found in the LICENSE file.

.. meta::
    :description: Install guide for tsuru clients
    :keywords: paas, cloud computing, tsuru

++++++++++++++++++++++++
Installing tsuru clients
++++++++++++++++++++++++

tsuru contains three clients: ``tsuru``, ``tsuru-admin`` and ``crane``.

* **tsuru** is the command line utility used by application developers, that
  will allow users to create, list, bind and manage apps. For more details,
  check :doc:`tsuru usage </apps/client/usage>`;
* **crane** is used by service administrators. For more detail, check
  :doc:`crane usage </services/usage>`;
* **tsuru-admin** is used by cloud administrators. Whoever is allowed to use
  it has gotten super powers :-)

This document describes how you can install those clients, using pre-compiled
binaries or building them from source.

`Using homebrew (Mac OS X only)`_

`Pre-built binaries (Linux and Mac OS X)`_

`Build from source (Linux and Mac OS X)`_

Using homebrew (Mac OS X only)
==============================

If you use Mac OS X and `homebrew <http://mxcl.github.com/homebrew/>`_, you may
use a custom tap to install ``tsuru``, ``crane`` and ``tsuru-admin``. First you
need to add the tap:

.. highlight:: bash

::

    $ brew tap globocom/homebrew-tsuru

Now you can install tsuru, tsuru-admin and crane:

.. highlight:: bash

::

    $ brew install tsuru
    $ brew install tsuru-admin
    $ brew install crane

Whenever a new version of any of tsuru's clients is out, you can just run:

.. highlight:: bash

::

    $ brew update
    $ brew upgrade <formula> # tsuru/tsuru-admin/crane

For more details on taps, check `homebrew documentation
<https://github.com/mxcl/homebrew/wiki>`_.

**NOTE:** Tsuru requires Go 1.1 or higher. Make sure you have the last version
of Go installed in your system.

Pre-built binaries (Linux and Mac OS X)
=======================================

tsuru clients are also distributed in binary version, so you can just download
an executable and put them somewhere in your ``PATH``.

It's important to note that all binaries are platform dependent. Currently, we
provide each of them in three flavors:

#. **darwin_amd64**: This is Mac OS X, 64 bits. Make sure the command ``uname -ms``
   prints "Darwin x86_64", otherwise this binary will not work in your system;
#. **linux_386**: This is Linux, 32 bits. Make sure the command ``uname -ms``
   prints "Linux x86", otherwise this binary will not work in your system;
#. **linux_amd64**: This is Linux, 64 bits. Make sure the command ``uname -ms``
   prints "Linux x86_64", otherwise this binary will not work in your system.

Below are the links to the binaries, you can just download, extract the archive
and put the binary somewhere in your PATH:

**darwin_amd64**

* tsuru: https://s3.amazonaws.com/tsuru/dist-cmd/tsuru-darwin-amd64.tar.gz
* tsuru-admin: https://s3.amazonaws.com/tsuru/dist-cmd/tsuru-admin-darwin-amd64.tar.gz
* crane: https://s3.amazonaws.com/tsuru/dist-cmd/crane-darwin-amd64.tar.gz

**linux_386**

* tsuru: https://s3.amazonaws.com/tsuru/dist-cmd/tsuru-linux-386.tar.gz
* tsuru-admin: https://s3.amazonaws.com/tsuru/dist-cmd/tsuru-admin-linux-386.tar.gz
* crane: https://s3.amazonaws.com/tsuru/dist-cmd/crane-linux-386.tar.gz

**linux_amd64**

* tsuru: https://s3.amazonaws.com/tsuru/dist-cmd/tsuru-linux-amd64.tar.gz
* tsuru-admin: https://s3.amazonaws.com/tsuru/dist-cmd/tsuru-admin-linux-amd64.tar.gz
* crane: https://s3.amazonaws.com/tsuru/dist-cmd/crane-linux-amd64.tar.gz

Build from source (Linux and Mac OS X)
======================================

`Tsuru's source <https://github.com/globocom/tsuru>`_ is written in `Go
<http://golang.org>`_, so before installing tsuru from source, please make sure
you have `installed and configured Go <http://golang.org/doc/install>`_.

With Go installed and configured, you can use ``go get`` to install any of
tsuru's clients:

.. highlight:: bash

::

    $ go get github.com/globocom/tsuru/cmd/tsuru
    $ go get github.com/globocom/tsuru/cmd/tsuru-admin
    $ go get github.com/globocom/tsuru/cmd/crane
