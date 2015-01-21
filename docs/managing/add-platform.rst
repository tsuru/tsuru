.. Copyright 2014 tsuru authors. All rights reserved.
   Use of this source code is governed by a BSD-style
   license that can be found in the LICENSE file.


++++++++++++++++++++
Installing platforms
++++++++++++++++++++

A platform is a well defined pack with installed dependencies for a language or
framework that a group of applications will need.

Platforms are defined as Dockerfiles and tsuru already have a number of supported
ones listed in `https://github.com/tsuru/basebuilder
<https://github.com/tsuru/basebuilder>`_

These platforms don't come pre-installed in tsuru, you have to add them to your
server using the `platform-add <http://tsuru-admin.readthedocs.org/en/latest/#platform-add>`_ command in
:doc:`tsuru-admin </reference/tsuru-admin>`.

.. highlight:: bash

::

    tsuru-admin platform-add platform-name --dockerfile dockerfile-url

For example, to install the Python platform from tsuru's basebuilder repository
you simply have to call:

.. highlight:: bash

::

    tsuru-admin platform-add python --dockerfile https://raw.githubusercontent.com/tsuru/basebuilder/master/python/Dockerfile


.. attention::

    If you have more than one docker node, you may use `docker-registry <https://github.com/docker/docker-registry>`_
    to add and distribute your platforms among your docker nodes.

    You can use the official `docker registry <https://registry.hub.docker.com/>`_ or install it by yourself.
    To do this you should first have to install `docker-registry <https://github.com/docker/docker-registry>`_ in any
    server you have. It should have a public ip to communicate with your docker nodes.

    Then you should `add registry address to tsuru.conf <http://docs.tsuru.io/en/latest/reference/config.html#docker-registry>`_.

