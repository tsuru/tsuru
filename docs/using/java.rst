.. Copyright 2014 tsuru authors. All rights reserved.
   Use of this source code is governed by a BSD-style
   license that can be found in the LICENSE file.

++++++++++++++++++++++++++++++++++++
Deploying Java applications on tsuru
++++++++++++++++++++++++++++++++++++

Overview
========

This document is a hands-on guide to deploying a simple Java application on
tsuru. The example application is a simple mvn generated archetype, in order to
generate it, just run:

.. highlight: bash

::

    $ mvn archetype:generate -DgroupId=io.tsuru.javasample -DartifactId=helloweb -DarchetypeArtifactId=maven-archetype-webapp

You can also deploy any other Java application you have on a tsuru server.
Another alternative is to just download the code available at GitHub:
https://github.com/tsuru/tsuru-java-sample.

Creating the app within tsuru
=============================

To create an app, you use `app-create
<http://godoc.org/github.com/tsuru/tsuru-client/tsuru#hdr-Create_an_app>`_
command:

.. highlight:: bash

::

    $ tsuru app-create <app-name> <app-platform>

For Java, the app platform is, guess what, ``java``! Let's call our application "helloweb":

.. highlight:: bash

::

    $ tsuru app-create helloweb java

To list all available platforms, use `platform-list
<http://godoc.org/github.com/tsuru/tsuru-client/tsuru#hdr-Display_the_list_of_available_platforms>`_
command.

You can see all your applications using `app-list
<http://godoc.org/github.com/tsuru/tsuru-client/tsuru#hdr-List_apps_that_you_have_access_to>`_
command:

.. highlight:: bash

::

    $ tsuru app-list
    +-------------+-------------------------+-------------------------------------+--------+
    | Application | Units State Summary     | Address                             | Ready? |
    +-------------+-------------------------+-------------------------------------+--------+
    | helloweb    | 0 of 0 units in-service | http://helloweb.tsuru.mycompany.com | No     |
    +-------------+-------------------------+-------------------------------------+--------+

Once your app is ready, you will be able to deploy your code, e.g.:

.. highlight:: bash

::

    $ tsuru app-list
    +-------------+-------------------------+-------------------------------------+--------+
    | Application | Units State Summary     | Address                             | Ready? |
    +-------------+-------------------------+-------------------------------------+--------+
    | helloweb    | 0 of 0 units in-service | http://helloweb.tsuru.mycompany.com | Yes    |
    +-------------+-------------------------+-------------------------------------+--------+

Deploying the code
==================

Using the Java platform, there are two deployment strategies: users can either
upload WAR files to tsuru or send the code using the regular ``git push``
approach. This guide will cover both approaches:

WAR deployment
--------------

Using the mvn archetype, generating the WAR is as easy as running ``mvn
package``, then the user can deploy the code using ``tsuru app-deploy``:

.. highlight:: bash

::

    $ mvn package
    $ cd target
    $ tsuru app-deploy -a helloweb helloweb.war
    Uploading files...... ok

    ---- Building application image ----
     ---> Sending image to repository (0.00MB)
     ---> Cleaning up

    ---- Starting 1 new unit ----
     ---> Started unit d2811c0801...

    ---- Adding routes to 1 new units ----
     ---> Added route to unit d2811c0801

    OK

Done! Now you can access your project in the address given by tsuru. Remeber to
add ``/helloweb/``. Something like
http://helloweb.tsuru.mycompany.com/helloweb/.

You can also deploy you application to the / address, renaming the WAR to
ROOT.war and redeploying it:

.. highlight:: bash

::

    $ mv helloweb.war ROOT.war
    $ tsuru app-deploy -a helloweb ROOT.war
    Uploading files... ok

    ---- Building application image ----
     ---> Sending image to repository (0.00MB)
     ---> Cleaning up

    ---- Starting 1 new unit ----
     ---> Started unit 4d155e805f...

    ---- Adding routes to 1 new units ----
     ---> Added route to unit 4d155e805f

    ---- Removing routes from 1 old units ----
     ---> Removed route from unit d2811c0801

    ---- Removing 1 old unit ----
     ---> Removed old unit 1/1

    OK

And now you can access your hello world in the root of the application address!

Git deployment
--------------

For Git deployment, we will send the code to tsuru, and compile the classes
there. For that, we're going to use mvn with the `Jetty plugin
<https://www.eclipse.org/jetty/documentation/current/jetty-maven-plugin.html>`_.
For doing that, we will need to create a Procfile with the command for starting
the application:

.. highlight:: bash

::

    $ cat Procfile
    helloweb: mvn jetty:run

In order to compile the application classes during deployment, we need also to
add a deployment hook. tsuru parses a file called ``tsuru.yaml`` and runs some
build hooks in the deployment phase.

Here is how the file for the ``helloweb`` application looks like:


.. highlight:: bash

::

    $ cat tsuru.yaml
    hooks:
      build:
        - mvn package

After adding these files, we're ready for deploying the application. The
`app-info
<http://godoc.org/github.com/tsuru/tsuru-client/tsuru#hdr-Display_information_about_an_app>`_
command will display a Git remote that we can use to push the application code
to production:


.. highlight:: bash

::

    $ tsuru app-info -a helloweb
    Application: helloweb
    Repository: git@tsuru.mycompany.com:helloweb.git
    Platform: java
    Teams: supercompany
    Address: helloweb.tsuru.mycompany.com
    Owner: user@company.com
    Team owner: supercompany
    Deploys: 2
    Units: 1
    +------------+---------+
    | Unit       | State   |
    +------------+---------+
    | d8a2d14948 | started |
    +------------+---------+

The "Repository" line contains what we need: the remote repository. Now we can
simply push the application code, using git push:


.. highlight:: bash

::

    $ git push git@tsuru.mycompany.com:helloweb.git master
    Counting objects: 25, done.
    Delta compression using up to 4 threads.
    Compressing objects: 100% (19/19), done.
    Writing objects: 100% (25/25), 2.59 KiB | 0 bytes/s, done.
    Total 25 (delta 5), reused 0 (delta 0)
    remote: tar: Removing leading `/' from member names
    remote: [INFO] Scanning for projects...
    remote: [INFO]
    remote: [INFO] ------------------------------------------------------------------------
    remote: [INFO] Building helloweb Maven Webapp 1.0-SNAPSHOT
    remote: [INFO] ------------------------------------------------------------------------
    remote: Downloading: http://repo.maven.apache.org/maven2/org/apache/maven/plugins/maven-resources-plugin/2.3/maven-resources-plugin-2.3.pom
    remote: Downloaded: http://repo.maven.apache.org/maven2/org/apache/maven/plugins/maven-resources-plugin/2.3/maven-resources-plugin-2.3.pom (5 KB at 6.0 KB/sec)
    remote: Downloading: http://repo.maven.apache.org/maven2/org/apache/maven/plugins/maven-plugins/12/maven-plugins-12.pom
    remote: Downloaded: http://repo.maven.apache.org/maven2/org/apache/maven/plugins/maven-plugins/12/maven-plugins-12.pom (12 KB at 35.9 KB/sec)

    ...

    remote: [INFO] Packaging webapp
    remote: [INFO] Assembling webapp [helloweb] in [/home/application/current/target/helloweb]
    remote: [INFO] Processing war project
    remote: [INFO] Copying webapp resources [/home/application/current/src/main/webapp]
    remote: [INFO] Webapp assembled in [27 msecs]
    remote: [INFO] Building war: /home/application/current/target/helloweb.war
    remote: [INFO] WEB-INF/web.xml already added, skipping
    remote: [INFO] ------------------------------------------------------------------------
    remote: [INFO] BUILD SUCCESS
    remote: [INFO] ------------------------------------------------------------------------
    remote: [INFO] Total time: 51.729s
    remote: [INFO] Finished at: Tue Nov 11 17:04:05 UTC 2014
    remote: [INFO] Final Memory: 8M/19M
    remote: [INFO] ------------------------------------------------------------------------
    remote:
    remote: ---- Building application image ----
    remote:  ---> Sending image to repository (2.96MB)
    remote:  ---> Cleaning up
    remote:
    remote: ---- Starting 1 new unit ----
    remote:  ---> Started unit e71d176232...
    remote:
    remote: ---- Adding routes to 1 new units ----
    remote:  ---> Added route to unit e71d176232
    remote:
    remote: ---- Removing routes from 1 old units ----
    remote:  ---> Removed route from unit d8a2d14948
    remote:
    remote: ---- Removing 1 old unit ----
    remote:  ---> Removed old unit 1/1
    remote:
    remote: OK
    To git@tsuru.mycompany.com:helloweb.git
     * [new branch]      master -> master

As you can see, the final part of the output is the same, and the application
is running in the address given by tsuru as well.

Switching between Java versions
===============================

In the Java platform provided by tsuru, users can use two version of Java: 7
and 8, both provided by Oracle. There's an environment variable for defining
the Java version you wanna use: ``JAVA_VERSION``. The default behavior of the
platform is to use Java 7, but you can change to Java 8 by running:

.. highlight:: bash

::

    $ tsuru env-set -a helloweb JAVA_VERSION=8
    ---- Setting 1 new environment variables ----

    ---- Starting 1 new unit ----
     ---> Started unit d8a2d14948...

    ---- Adding routes to 1 new units ----
     ---> Added route to unit d8a2d14948

    ---- Removing routes from 1 old units ----
     ---> Removed route from unit 4d155e805f

    ---- Removing 1 old unit ----
     ---> Removed old unit 1/1

And... done! No need to run another deployment, your application is now running
with Java 8.

Going further
=============

For more information, you can dig into `tsuru docs <http://docs.tsuru.io>`_, or
read `complete instructions of use for the tsuru command
<http://godoc.org/github.com/tsuru/tsuru-client/tsuru>`_.
