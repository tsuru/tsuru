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

    $ tsuru service-add <service_name>

To remove a instance of a service:

.. highlight:: bash

::

    $ tsuru service-remove <service_instance_name>

To bind a service instance with an app you can use the `bind` command.
If this service has any variable to be used for your app, tsuru will inject this variables in environment.

.. highlight:: bash

::

    $ tsuru bind <service_instance_name> <app_name>

And to unbind:

.. highlight:: bash

::

    $ tsuru unbind <service_instance_name> <app_name>
