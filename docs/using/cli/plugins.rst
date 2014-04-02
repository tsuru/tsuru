Guide to create tsuru cli plugins 
============================


Creating your own plugin
------------------------

Everything you need to do is to create a new `executable`. You can use bash, python, ruby, eg.

Let's create a `Hello world` plugin that prints `hello world` as output. Let's use `bash` to write our new plugin.

#! /bin/bash
echo "hello world!"

Install your plugin
-------------------

Let's install your plugin. There are two ways to install. The first way is to move your plugin to `$HOME/.tsuru/plugins`. The other way is to use `tsuru plugin-install`.

`tsuru plugin-install` will download your plugin file to `$HOME/.tsuru/plugins`. The `tsuru plugin-install` sintaxe is:

$ tsuru plugin-install http://yourplugin.com

Executing your plugin
---------------------

To execute your plugin just follow this pattern `tsuru pluginname <args>`. Applying it to our `hello world` example:

$ tsuru hello-world
hello world!
