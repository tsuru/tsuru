#Tsuru

##What is Tsuru?

Tsuru is an open Platform-as-a-Service (PaaS).

##Installation

Please check the [INSTALL](/timeredbull/tsuru/blob/master/INSTALL.md) file for
installation instructions.

##Server configuration

TODO!

##Usage

After installing the server, build the cmd/main.go file with the name you wish,
and add it to your $PATH. Here we'll call it `tsuru`. Then you must set the
target with your server url (tsuru target), like:

    % tsuru target tsuru.myhost.com
    % tsuru target localhost:8080

After that, all you need is create a user and authenticate to start creating
apps and pushing code to them:

    % tsuru user create youremail@gmail.com
    % tsuru login youremail@gmail.com

Every command has a help, to access it, try:

    % tsuru help command

Sometimes you also need help with a subcommand:

    % tsuru help command subcommand

Then, create a team:

    % tsuru team create myteam

Your user will be automatically added to this team.

To create an app:

    % tsuru app create myapp [framework]

The 'framework' parameter is optional, and defaults to 'django'. The available frameworks are listed at https://github.com/timeredbull/charms.

The command output will return your the git remote url for your app, you should add it to
your git repository:

    % git remote add tsuru git@tsuru.myhost.com:myapp.git

When your app is ready, you can push to it. To check whether it is ready or
not, you can use:

    % tsuru app list

This will return something like:

    +-------------+---------+--------------+
    | Application | State   | Ip           |
    +-------------+---------+--------------+
    | myapp       | STARTED | 10.10.10.10  |
    +-------------+---------+--------------+

You can try to push now, but you'll get a permission error, because you haven't
pushed your key yet.

    % tsuru key add

This will search for a `id_rsa.pub` file in ~/.ssh/, if you don't have a
generated key yet, you should generate one before running this command.

Now you can push you application to your cloud:

    % git push tsuru master

After that, you can check your app's url in the browser and see your app there.
You'll probably need run migrations or other deploy related commands.

To run a single command, you should use the command line:

    % tsuru run myapp env/bin/python manage.py syncdb
    % tsuru run myapp env/bin/python manage.py migrate

By default, the commands are run from inside the app root directory, which is
/home/application. If you have more complicated deploy related commands, you
should use the app.conf pre-restart and pos-restart scripts, those are run
before and after the restart of your app, which is triggered everytime you push
code.

Below is a app.conf sample::

```yaml
pre-restart: deploy/pre.sh
pos-restart: deploy/pos.sh
```

The `app.conf` file is located in your app's root directory, and the scripts
path in the yaml are relative to it.
