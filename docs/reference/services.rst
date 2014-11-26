.. Copyright 2014 tsuru authors. All rights reserved.
   Use of this source code is governed by a BSD-style
   license that can be found in the LICENSE file.

++++++++
Services
++++++++

You can manage your services using the tsuru command-line interface.

To list all services avaliable you can use, you can use the `service-list
<http://godoc.org/github.com/tsuru/tsuru-client/tsuru#hdr-List_available_services_and_instances>`_
command:

.. highlight:: bash

::

    $ tsuru service-list

To add a new instance of a service, use the `service-add
<http://godoc.org/github.com/tsuru/tsuru-client/tsuru#hdr-Create_a_new_service_instance>`_
command:

.. highlight:: bash

::

    $ tsuru service-add <service_name> <service_instance_name>

To remove an instance of a service, use the `service-remove
<http://godoc.org/github.com/tsuru/tsuru-client/tsuru#hdr-Remove_a_service_instance>`_
command:

.. highlight:: bash

::

    $ tsuru service-remove <service_instance_name>

To bind a service instance with an app you can use the `service-bind
<http://godoc.org/github.com/tsuru/tsuru-client/tsuru#hdr-Bind_an_application_to_a_service_instance>`_
command.  If this service has any variable to be used by your app, tsuru will
inject this variables in the app's environment.

.. highlight:: bash

::

    $ tsuru service-bind <service_instance_name> [--app appname]

And to unbind, use `service-unbind
<http://godoc.org/github.com/tsuru/tsuru-client/tsuru#hdr-Unbind_an_application_from_a_service_instance>`_
command:

.. highlight:: bash

::

    $ tsuru service-unbind <service_instance_name> [--app appname]

For more details on the ``--app`` flag, see `"Guessing app names"
<http://godoc.org/github.com/tsuru/tsuru-client/tsuru#hdr-Guessing_app_names>`_
section of tsuru command documentation.
