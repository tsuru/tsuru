.. Copyright 2012 tsuru authors. All rights reserved.
   Use of this source code is governed by a BSD-style
   license that can be found in the LICENSE file.

.. meta::
    :description: Install guide for tsuru client
    :keywords: paas, cloud computing, tsuru

++++++++++++++++++++++++++
tsuru client install guide
++++++++++++++++++++++++++

You can download the client binary for your platform and put it in your path. All binaries are available in the `downloads <https://github.com/globocom/tsuru/downloads>`_ page.

At this moment, we provide two versions of the client, for Linux amd64 and Darwin amd64.

Linux example: suppose you want to install the tsuru client in your `/usr/bin` directory, you can run:

.. highlight:: bash

::

    $ curl -sL https://github.com/downloads/globocom/tsuru/tsuru-linux-amd64-0.2.1.tar.gz | sudo tar -xz -C /usr/bin/

Then you will be able to :doc:`use the client </apps/client/usage>`. On Mac OS, use `darwin` instead of `linux`:

.. highlight:: bash

::

    $ curl -sL https://github.com/downloads/globocom/tsuru/tsuru-darwin-amd64-0.2.1.tar.gz | sudo tar -xz -C /usr/bin/
