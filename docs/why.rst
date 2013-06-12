Why Tsuru?
==========

This document aims to show Tsuru's most killing features. Additionally, provides a comparison of Tsuru
with others PaaS's on the market.

Easy Server Installation
------------------------

It's really easy to have a running PaaS with Tsuru. We provide a serie of scripts, each one built to install
and configure the required components for each Tsuru provisioner, you can check our scripts on
`Tsuru repository <https://github.com/globocom/tsuru/tree/master/misc>`_, there are separeted scripts to install each
component, so it's easy to create your own script to configure a new provisioner or to change the configuration of
an existing one.

But it's okay if you want more control and do not want to use our scripts, or want to better understand the interaction
between Tsuru components, we built `a guide <http://docs.tsuru.io/en/latest/build.html>`_ only for you.

Platforms Extensibility
-----------------------

One of Tsuru main goals is to be easily extensible. The platform is one great example of accomplishment on that.
Tsuru platforms works slightly different for each provisioner. Juju and LXC provisioners have exactly the same scripts
for platform provisioning (except for logging, the juju charms use juju-log commands, the LXC don't), you can find
the scripts on our `charms repository <https://github.com/globocom/charms>`_, Juju charms are on branch master and LXC
on the lxc branch. The Docker provisioner is a bit different, it has an specific image for each platform, if one wants
to create a new platform, just extend tsuru/base image and follow the directory tree structure, the scripts and
Dockerfile for our existing platforms images can be found on our `images repository <https://github.com/flaviamissi/basebuilder>`_

Services Creation and Extension
-------------------------------

Most applications need a service to work properly, like a database service. Tsuru provides an interface API to communicate
with services APIs, but it doesn't manage services directly. We made this decision so service managers could freely
manage everything about their services without Tsuru interfering with it.

In order to create a new service one should write an API implementing the predefined endpoints that Tsuru will call when
a user performs an action using the client, read more on the `Building your service tutorial <http://docs.tsuru.io/en/latest/services/build.html>`_.

One may also change an existing service, if its source is open. All services APIs made by Tsuru team are open and you can
fork them and make any changes one want. For example, the mongoDB api shares one database installation with everyone that is using it,
if you don't like it and want to change it, you can do it and create a new service on Tsuru with your own implementation.

IaaS's and Provisioners
-----------------------

Tsuru provides an easy way to change the application unit provisioning system. Tsuru already has three
working provisioners, `Juju <https://juju.ubuntu.com/>`_, `Docker <http://www.docker.io/>`_ and `LXC <http://lxc.sourceforge.net/>`_.
But the main advantage is the ease of extending the provisioning system. One can simply implement
the Provision interface Tsuru provides, configure it on a Tsuru installation and start using it.

Routers
-------

Tsuru also provides an abstraction for routing control and load balancing in application units.
It provides a routing interface, that you can combine on many ways: you can plug any router with any provisioner,
you can also create your own routing backend and plug it with any existing provisioner, this can be done
only changing Tsuru's configuration file.

Comparing Tsuru With Other PaaS's
---------------------------------

The following table compares Tsuru with OpenShift and Stackato PaaS's.

If you have anything to consider, or want to ask us to add another PaaS on the list
contact us in #tsuru @ freenode.net or at our `mailing list <https://groups.google.com/d/forum/tsuru-users>`_

+-------------------------+------------------------+--------------------+----------------------+
|                         | Tsuru                  | OpenShift          | Stackato             |
+=========================+========================+====================+======================+
| Built-in Platforms      | Node.js, PHP,          | Java, PHP,         | Java, Node.Js,       |
|                         | HTML, Python, Ruby,    | Ruby, Node.js,     | Perl, PHP,           |
|                         | Golang, Java           | Python             | Python, Ruby         |
+-------------------------+------------------------+--------------------+----------------------+
| End-user web UI         | Yes (Abyss)            | Yes                | Yes                  |
+-------------------------+------------------------+--------------------+----------------------+
| CLI                     | Yes                    | Yes                | Yes                  |
+-------------------------+------------------------+--------------------+----------------------+
| Deployment hooks        | Yes                    | No                 | Yes                  |
+-------------------------+------------------------+--------------------+----------------------+
| SSH Access              | No                     | Yes                | Yes                  |
+-------------------------+------------------------+--------------------+----------------------+
| Run Commands Remotely   | Yes                    | No                 | No                   |
+-------------------------+------------------------+--------------------+----------------------+
| Application Monitoring  | Yes                    | Yes                | Yes                  |
+-------------------------+------------------------+--------------------+----------------------+
| SQL Databases           | MySQL                  | MySQL, PostgreSQL  | MySQL, PostgreSQL    |
+-------------------------+------------------------+--------------------+----------------------+
| NoSQL Databases         | MongoDB, Cassandra     | MongoDB            | MongoDB, Redis       |
|                         | Memcached, Redis       |                    |                      |
+-------------------------+------------------------+--------------------+----------------------+
| Log Streaming           | Yes                    | Yes (not built-in) | Yes                  |
+-------------------------+------------------------+--------------------+----------------------+
| Metering/Billing API    | No                     | No                 | Yes                  |
+-------------------------+------------------------+--------------------+----------------------+
| Quota System            | Yes                    | Yes                | Yes                  |
+-------------------------+------------------------+--------------------+----------------------+
| Container Based Apps    | Yes                    | Yes                | Yes                  |
+-------------------------+------------------------+--------------------+----------------------+
| VMs Based Apps          | Yes                    | No                 | No                   |
+-------------------------+------------------------+--------------------+----------------------+
| Open Source             | Yes                    | Yes                | No                   |
+-------------------------+------------------------+--------------------+----------------------+
| Free                    | Yes                    | Yes                | Yes                  |
+-------------------------+------------------------+--------------------+----------------------+
| Paid/Closed Version     | No                     | Yes                | Yes                  |
+-------------------------+------------------------+--------------------+----------------------+
| PaaS Healing            | Yes                    | No                 | No                   |
+-------------------------+------------------------+--------------------+----------------------+
| App Healing             | Yes                    | No                 | No                   |
+-------------------------+------------------------+--------------------+----------------------+
| App Fault Tolerance     | Yes                    | Yes (by cartridge) | Yes                  |
+-------------------------+------------------------+--------------------+----------------------+
| Auto Scaling            | No                     | Yes                | Yes (for some IaaSs) |
+-------------------------+------------------------+--------------------+----------------------+
| Manual Scaling          | Yes                    | No                 | Yes                  |
+-------------------------+------------------------+--------------------+----------------------+
