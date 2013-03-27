Tsuru Overview
==============

Tsuru is an OpenSource PaaS. If you don't know what a PaaS is and what it does, see `wikipidia's description <http://en.wikipedia.org/wiki/PaaS>`_.

It follows the principles described in the `The Twelve-Factor App <http://www.12factor.net/>`_ methodology.

Immediate Deploy
----------------

Deploying an app is simple and easy. No special tools needed, just a plain git push. Deployment is instant, whether your app is big or small.

Tsuru will take care of all the applications dependencies. You can specify all kinds of dependecies: Operational System and language specific ones.
For example, if you have a Python application, tsuru will search for the requirements.txt file, but first it will search for OS dependencies. in Ubuntu, for instance,
Tsuru will search for a file called requirements.apt, after the installation of the packages listed there, it'll install the language dependencies, in this example
running `pip instal` passing the requirements.txt file.

Continuos Deploy
----------------

Easily create testing, staging, and production versions of your app and deploy to and between them instantly.

Add-on Resources
----------------

Instantly provision and integrate third party services with one command. It's so easy to create a service in Tsuru that you'll want to sell your own services.
Tsuru provides the basic services your application will need. Databases, Search, Caching, Storage, FrontEnd, you can get all of that in a fashionable and really easy way.

Tsuru talks with services using a well defined API, you just have to implement four endpoints that implement the logic of provisioning your service to an application
(like creating VMs, liberating access, etc) and register your service with a really simple yaml formatted manifest. It's really simple.

Per-Deploy Config Variables
---------------------------

Configuration for an application should be stored in environment variables - and we know that. Tsuru lets you define your environment variables using the command line,
so you can have the configuration flexibility your application need.

Additionaly, when you bind a service with your application, Tsuru gives the service the hability to inject environment variables in your application environment.
For instance, if you use our Mysql service, it will inject variables for you to create a connection with your application database.

Custom Services
---------------

Tsuru already has services for you to use, but you don't need to use them at all if you don't want to. Simply configure environment variables and use them
in your application config

Logging and Visibility
----------------------

Full visibility into your app's operations with real-time logging, process status inspection, and an audit trail of all releases.
Tsuru will log everything related to your application, and you can check it out with only one command: `tsuru log`. You can filter logs, for example,
if you don't want to see the logs of developers activity, you can specify the source as app and you'll get only the application webserver logs.

Process Management
------------------

Tsuru manages the application process for your application, but it does not know to start it. You can teach Tsuru how to start your application using
a Procfile. Tsuru reads the Procfile and uses Circus_ to manage your application process. You can even enable a web console for circus to manage your
application process and to watch CPU and memory usage in real-time.

Tsuru allows you to easily restart your application process by the command line. But you generaly does not need to do that, Tsuru will take care of everything
for you.

.. _Circus: http://circus.readthedocs.org

Control Surfaces
----------------

Tsuru offers two main ways of interacting with it: the CLI interface, which is alsome for day-to-day usage, and the web interface, which is in alpha,
but it is also a great tool to create, check logs and monitor applications and services resources.
Of course we have an solid API, so if you want to make your own tools, it's also possible!

Scaling
-------

The Juju_ provisioner allows you to easily add and remove units, easily allowing one to scale an application.

Tsuru also has a FrontEnd as a Service powered by varnish. One application may have a whole farm of varnish VMs receiving its traffic.

Collaboration
-------------

Manage sharing and deployment of your application. Tsuru has teams concept, you can create your team and add your teammates on tsuru.
One can be on various teams and control which applications the teams has permissions.

Easy Server Deployment
----------------------

Tsuru is really easy to deploy, you can get it done by following these simple steps. (list here)

Distributed and Extensible
--------------------------

Tsuru server is easily extensible and customizable. By default, it will deploy applications using Juju_, but you can easily implement your own
provisioner and use whenever backend you wish.

When you extend Tsuru, you are praticaly building a new PaaS using Tsuru's structure. You change the whole Tsuru workflow by implementing a new provider.
Tsuru allows it with the power of Golang, it uses interfaces, so you can just create your own provisioner respecting Tsuru's interface and plug in it, changing your PaaS
behavior.

.. _Juju: https://juju.ubuntu.com/

Dev/Ops Perspective
-------------------

Tsuru's components are distributed, it is formed by various pieces of software, each one made to be easily deployed and maintained.

Application Developer Perspective
---------------------------------

We aim to make developers life easier.
