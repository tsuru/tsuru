.. meta::
    :description: Install guide for tsuru client
    :keywords: paas, cloud computing, tsuru

++++++++++++++++++++++++++
tsuru client install guide
++++++++++++++++++++++++++

You can download the client binary for your platform and put it in your path. All binaries are available in the `downloads <https://github.com/timeredbull/tsuru/downloads>`_ page.

At this moment, we provide two versions of the client, for Linux amd64 and Darwin amd64.

Linux example: suppose you want to install the tsuru client in your `/usr/bin` directory, you can run:

.. highlight:: bash

::

    $ curl -4L https://github.com/downloads/timeredbull/tsuru/tsuru-linux-amd64 -o /usr/bin/tsuru

Then you will be able to [use the client](apps/client/usage.html). On Mac OS, use `darwin` instead of `linux`:

.. highlight:: bash

::

    $ curl -4L https://github.com/downloads/timeredbull/tsuru/tsuru-darwin-amd64 -o /usr/bin/tsuru
