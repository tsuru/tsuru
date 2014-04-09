.. Copyright 2014 tsuru authors. All rights reserved.
   Use of this source code is governed by a BSD-style
   license that can be found in the LICENSE file.

+++++++++++++++++++++++++
Server installation guide
+++++++++++++++++++++++++

Dependencies
============

tsuru depends on `Go <http://golang.org>`_ and `libyaml <http://pyyaml.org/wiki/LibYAML>`_.

To install Go, follow the official instructions in the language website:
http://golang.org/doc/install.

To install libyaml, you can use one package manager, or download it and install
it from source. To install from source, follow the instructions on PyYAML wiki:
http://pyyaml.org/wiki/LibYAML.

The following instructions are system specific:

FreeBSD
-------

.. highlight:: bash

::

    $ cd /usr/ports/textproc/libyaml
    $ make install clean

Mac OS X (homebrew)
-------------------

.. highlight:: bash

::

    $ brew install libyaml

Ubuntu
------

.. highlight:: bash

::

    $ [sudo] apt-get install libyaml-dev

CentOS
------

.. highlight:: bash

::

    $ [sudo] yum install libyaml-devel

Installation
============

After installing and configuring go, and installing libyaml, just run in your terminal:

.. highlight:: bash

::

    $ go get github.com/tsuru/tsuru/...

Server configuration
====================

TODO!
