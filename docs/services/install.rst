.. Copyright 2012 tsuru authors. All rights reserved.
   Use of this source code is governed by a BSD-style
   license that can be found in the LICENSE file.

++++++++++++++++++++++++++
crane client install guide
++++++++++++++++++++++++++

You can download the client binary for your platform and put it in your path. All binaries are available in the `downloads <https://github.com/timeredbull/crane/downloads>`_ page.

At this moment, we provide two versions of the client, for Linux amd64 and Darwin amd64.

Linux example: suppose you want to install the crane client in your `/usr/bin` directory, you can run:

.. highlight:: bash

::

    $ sudo curl -L https://github.com/downloads/globocom/tsuru/20120905-crane-linux-amd64.tar.gz | sudo tar -xz -C /usr/bin/

Then you will be able to :doc:`use the client </services/usage>`. On Mac OS, use `darwin` instead of `linux`:

.. highlight:: bash

::

    $ sudo curl -L https://github.com/downloads/globocom/tsuru/20120905-crane-darwin-amd64.tar.gz | sudo tar -xz -C /usr/bin/
