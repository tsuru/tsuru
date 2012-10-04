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

    $ sudo curl -L https://github.com/downloads/globocom/tsuru/20120905-tsuru-linux-amd64.tar.gz | sudo tar -xz -C /usr/bin/

Then you will be able to :doc:`use the client </apps/client/usage>`. On Mac OS, use `darwin` instead of `linux`:

.. highlight:: bash

::

    $ sudo curl -L https://github.com/downloads/globocom/tsuru/20120905-tsuru-darwin-amd64.tar.gz | sudo tar -xz -C /usr/bin/
