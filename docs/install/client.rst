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
   :doc:`crane usage </service/usage>`;
 * **tsuru-admin** is used by cloud administrators. Whoever is allowed to use
   it has gotten super powers :-)

This document describes how you can install those clients, using pre-compiled
binaries or building them from source.

Using homebrew (Mac OS X only)
==============================

If you use Mac OS X and `homebrew <http://mxcl.github.com/homebrew/>`_, you may
use a custom tap to install ``tsuru``, ``crane`` and ``tsuru-admin``. First you
need to add the tap:

.. highlight: bash

::

    $ brew tap globocom/homebrew-tsuru

Now you can install tsuru, tsuru-admin and crane:

.. highlight: bash

::

    $ brew install tsuru
    $ brew install tsuru-admin
    $ brew install crane

Whenever a new version of any of tsuru's clients is out, you can just run:

.. highlight: bash

::

    $ brew update
    $ brew upgrade <formula> # tsuru/tsuru-admin/crane

For more details on taps, check `homebrew documentation
<https://github.com/mxcl/homebrew/wiki>`_.
