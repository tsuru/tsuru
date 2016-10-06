.. Copyright 2015 tsuru authors. All rights reserved.
   Use of this source code is governed by a BSD-style
   license that can be found in the LICENSE file.

++++++++++++++++++++
Installing platforms
++++++++++++++++++++

A platform is a well defined pack with installed dependencies for a language or
framework that a group of applications will need.

Platforms are defined as Dockerfiles and tsuru already have a number of supported ones listed bellow:

- Go_
- Java_
- Nodejs_
- php_
- Python2_
- Python3_
- Ruby_
- Static_

.. _Static: https://github.com/tsuru/platforms/tree/master/static
.. _Ruby: https://github.com/tsuru/platforms/tree/master/ruby
.. _Python3: https://github.com/tsuru/platforms/tree/master/python3
.. _Python2: https://github.com/tsuru/platforms/tree/master/python
.. _php: https://github.com/tsuru/platforms/tree/master/php
.. _Nodejs: https://github.com/tsuru/platforms/tree/master/nodejs
.. _Java: https://github.com/tsuru/platforms/tree/master/java
.. _Go: https://github.com/tsuru/platforms/tree/master/go

These platforms don't come pre-installed in tsuru, you have to add them to your
server using the `platform-add
<http://tsuru-admin.readthedocs.org/en/latest/#platform-add>`_ command in
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

    If you have more than one Docker node, you may use `docker-registry
    <https://docs.docker.com/registry/>`_ to add and distribute your
    platforms among your Docker nodes.

    You can use the official `docker registry
    <https://registry.hub.docker.com/>`_ or install it by yourself. To do this
    you should first have to install `docker-registry
    <https://docs.docker.com/registry/>`_ in any server you have. It should
    have a public ip to communicate with your docker nodes.

    Then you should `add registry address to tsuru.conf
    <http://docs.tsuru.io/en/latest/reference/config.html#docker-registry>`_.
