Guide to create tsuru cli plugins
=================================

Installing a plugin
-------------------

Let's install a plugin. There are two ways to install. 
The first way is to move your plugin to `$HOME/.tsuru/plugins`.
The other way is to use `tsuru plugin-install` command.

`tsuru plugin-install` will download your plugin file to `$HOME/.tsuru/plugins`. 
The `tsuru plugin-install` sintaxe is:

.. highlight:: bash

::

    $ tsuru plugin-install <plugin-name> <plugin-url>

Listing installed plugins
-------------------------

To list all installed plugins you can use the `tsuru <plugin-list>` command:

.. highlight:: bash

::

    $ tsuru plugin-list
    plugin1
    plugin2

Executing a plugin
------------------

To execute a plugin just follow this pattern `tsuru <plugin-name> <args>`:

.. highlight:: bash

::

    $ tsuru <plugin-name>
    <plugin-output>

Removing a plugin
-----------------

To remove a plugin just use the `plugin-remove` command passing the `<plugin-name>` as argument:

.. highlight:: bash

::

    $ tsuru plugin-remove <plugin-name>
    Plugin "<plugin-name>" successfully removed!

Creating your own plugin
------------------------

Everything you need to do is to create a new `executable`. You can use bash, python, ruby, eg.

Let's create a `Hello world` plugin that prints `hello world` as output. Let's use `bash` to write our new plugin.

.. highlight:: bash

::

    #! /bin/bash
    echo "hello world!"

You can use the gist (gist.github.com) as host for your plugin.

To install your plugin you can use `tsuru plugin-install` command.
