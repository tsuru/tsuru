++++++++++
contribute
++++++++++

* Source hosted at `GitHub <http://github.com/timeredbull/tsuru>`_
* Report issues on `GitHub Issues <http://github.com/timeredbull/tsuru/issues>`_

Pull requests are very welcome! Make sure your patches are well tested and documented :)


running the tests
=================

You can use `make` to install all tsuru dependencies and run tests. It will also check if is everything ok with your `GOPATH` setup:

.. highlight:: bash

::

    $ make

writing docs
============

Tsuru documentation is written using `Sphinx <http://sphinx.pocoo.org/>`_, which uses `RST <http://docutils.sourceforge.net/rst.html>`_. Check these tools docs to learn how to write docs for Tsuru.

building docs
=============

In order to build the HTML docs, just run on terminal:

.. highlight:: bash

::

    $ make doc
