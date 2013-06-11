Why Tsuru?
==========

This document aims to show Tsuru's most killing features. Additionally, provides a comparison of Tsuru
with others PaaS's on the market.

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
