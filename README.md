#Tsuru

##What is Tsuru?

Tsuru is an open Platform-as-a-Service (PaaS).

##Dependencies

Tsuru depends on [Go](http://golang.org) and [libyaml](http://pyyaml.org/wiki/LibYAML).

To install Go, follow the official instructions in the language website:
http://golang.org/doc/install.

To install libyaml, you can use one package manager, or download it and install
from source. To install from source, follow the instructions on PyYAML wiki:
http://pyyaml.org/wiki/LibYAML.

The following instructions are system specific:

###FreeBSD

    % cd /usr/ports/textproc/libyaml
    % make install clean

###Mac OS X (homebrew)

    % brew install libyaml

###Ubuntu

    % [sudo] apt-get install libyaml-dev

###CentOS

    % [sudo] yum install libyaml-devel

##Installation

After install and configure go, and install libyaml, just run in your terminal:

    % go get github.com/timeredbull/tsuru/...

##Server configuration

TODO!

##Usage

After installing the server, build the cmd/main.go file with the name you wish, and add it to your $PATH. Here we'll call it `tsuru`.
Then you must set the target with your server url, like:

  `% tsuru target tsuru.myhost.com`

After that, all you need is create an user and authenticate to start creating apps and pushing code to them::

  `% tsuru user create youremail@gmail.com
   % tsuru login youremail@gmail.com`

To create an app:

  `% tsuru app create myblog`

This will return your app's remote url, you should add it to your git repository:

  `% git remote add tsuru git@tsuru.myhost.com:myblog.git`

When your app is ready, you can push to it. To check whether it is ready or not, you can use:

  `% tsuru app list`

This will return something like:

  `+-------------+---------+--------------+
  | Application | State   | Ip           |
  +-------------+---------+--------------+
  | myblog      | STARTED | 10.10.10.10  |
  +-------------+---------+--------------+`

You can try to push now, but you'll get a permission error, because you haven't pushed your key yet.

  `% tsuru key add`

This will search for a `id_rsa.pub` file in ~/.ssh/, if you don't have a generated key yet, you should generate one before running this command.
Now you can push you application to your cloud:

  `% git push tsuru master`

After that, you can check your app's url in the browser and see your app there. You'll probably need run migrations or other deploy related commands.
To run a single command, you should use the command line:

  `% tsuru app run myblog env/bin/python manage.py syncdb && env/bin/python manage.py migrate`

By default, the commands are run from inside the app root directory, which is /home/application. If you have more complicated deploy related commands,
you should use the app.conf pre-restart and pos-restart scripts, those are run before and after the restart of your app, which is triggered everytime you push code.
Below is a app.conf sample::

```yaml
pre-restart:
  deploy/pre.sh
pos-restart:
  deploy/pos.sh
```

The app.conf file is located in your app's root directory, and the scripts path in the yaml are relative to it.
