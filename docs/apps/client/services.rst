.. Copyright 2012 tsuru authors. All rights reserved.
   Use of this source code is governed by a BSD-style
   license that can be found in the LICENSE file.

++++++++
Services
++++++++

You can manage your services using the tsuru command-line interface.

To list all services avaliable you can use:

.. highlight:: bash

::

    $ tsuru service-list

To add a new instance of a service:

.. highlight:: bash

::

    $ tsuru service-add <service_name> <service_instance_name>

To remove an instance of a service:

.. highlight:: bash

::

    $ tsuru service-remove <service_instance_name>

To bind a service instance with an app you can use the `bind` command.
If this service has any variable to be used by your app, tsuru will inject this
variables in the app's environment.

.. highlight:: bash

::

    $ tsuru bind <service_instance_name> [--app appname]

And to unbind:

.. highlight:: bash

::

    $ tsuru unbind <service_instance_name> [--app appname]

For more details on the ``--app`` flag, see `"Guessing app names"
<http://godoc.org/github.com/globocom/tsuru/cmd/tsuru/developer#Guessing_app_names>`_
section of tsuru command documentation.
