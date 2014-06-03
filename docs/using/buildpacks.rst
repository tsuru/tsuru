Using Buildpacks
++++++++++++++++

tsuru supports deploying applications via Heroku Buildpacks.

Buildpacks are useful if you’re interested in following Heroku’s best practices for building applications or if you are deploying an application that already runs on Heroku.

tsuru uses `Buildstep Docker image <https://github.com/progrium/buildstep>`_ to make deploy using buildpacks possible.


Creating an Application
=======================

What do you need is create an application using `buildpack` platform:

.. highlight:: bash

::

    $ tsuru app-create myapp buildpack

Deploying your Application
==========================

Use git push master to deploy your application.

.. highlight:: bash

::

    $ git push <REMOTE-URL> master


Included Buildpacks
===================

A number of buildpacks come bundled by default:

* https://github.com/heroku/heroku-buildpack-ruby.git
* https://github.com/heroku/heroku-buildpack-nodejs.git
* https://github.com/heroku/heroku-buildpack-java.git
* https://github.com/heroku/heroku-buildpack-play.git
* https://github.com/heroku/heroku-buildpack-python.git
* https://github.com/heroku/heroku-buildpack-scala.git
* https://github.com/heroku/heroku-buildpack-clojure.git
* https://github.com/heroku/heroku-buildpack-gradle.git
* https://github.com/heroku/heroku-buildpack-grails.git
* https://github.com/CHH/heroku-buildpack-php.git
* https://github.com/kr/heroku-buildpack-go.git
* https://github.com/oortcloud/heroku-buildpack-meteorite.git
* https://github.com/miyagawa/heroku-buildpack-perl.git
* https://github.com/igrigorik/heroku-buildpack-dart.git
* https://github.com/rhy-jot/buildpack-nginx.git
* https://github.com/Kloadut/heroku-buildpack-static-apache.git
* https://github.com/bacongobbler/heroku-buildpack-jekyll.git
* https://github.com/ddollar/heroku-buildpack-multi.git

tsuru will cycle through the bin/detect script of each buildpack to match the code you are pushing.

Using a Custom Buildpack
========================

To use a custom buildpack, set the BUILDPACK_URL environment variable.

.. highlight:: bash

::

    $ tsuru env-set BUILDPACK_URL=https://github.com/dpiddy/heroku-buildpack-ruby-minimal

On your next git push, the custom buildpack will be used.

Creating your own Buildpack
===========================

You can follow this Heroku documentation to learn how to create your own Buildpack: https://devcenter.heroku.com/articles/buildpack-api. 
