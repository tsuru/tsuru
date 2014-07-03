.. Copyright 2014 tsuru authors. All rights reserved.
   Use of this source code is governed by a BSD-style
   license that can be found in the LICENSE file.

+++++++++++
Crane usage
+++++++++++

First, you must set the target with your server url, like:

.. highlight:: bash

::

    $ crane target tsuru.myhost.com

After that, all you need is to create a user and authenticate:

.. highlight:: bash

::

    $ crane user-create youremail@gmail.com
    $ crane login youremail@gmail.com

To generate a service template:

.. highlight:: bash

::

    $ crane template

This will create a manifest.yaml in your current path with this content:

.. highlight:: yaml

::

    id: servicename
    endpoint:
      production: production-endpoint.com
        test: test-endpoint.com:8080

The manifest.yaml is used by crane to define an id and an endpoint to your service.

To submit your new service, you can run:

.. highlight:: bash

::

    $ crane create path/to/your/manifest.yaml

To list your services:

.. highlight:: bash

::

    $ crane list

This will return something like:

.. highlight:: bash

::

    +----------+-----------+
    | Services | Instances |
    +----------+-----------+
    | mysql    | my_db     |
    +----------+-----------+

To update a service manifest:

.. highlight:: bash

::

    $ crane create path/to/your/manifest.yaml

To remove a service:

.. highlight:: bash

::

    $ crane remove service_name

It would be nice if your service had some documentation. To add a documentation to you service you can use:

.. highlight:: bash

::

    $ crane doc-add service_name path/to/your/docfile

Crane will read the content of the file and save it.

To show the current documentation of your service:

.. highlight:: bash

::

    $ crane doc-get service_name

Further instructions
====================

For a complete reference, check the documentation for crane command:
`<http://godoc.org/github.com/tsuru/crane>`_.
