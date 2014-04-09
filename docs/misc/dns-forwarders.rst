.. Copyright 2014 tsuru authors. All rights reserved.
   Use of this source code is governed by a BSD-style
   license that can be found in the LICENSE file.

+++++++++++++++++++++++++++++
Howto install a dns forwarder
+++++++++++++++++++++++++++++

This document describes how to create a dns forwarder and set a base domain for tsuru.

Overview
========

The recommended way to use tsuru is integrated with a DNS server.
The easiest way to do that is configuring it as a cache forwarder,
and configuring a DNS zone to be used for tsuru as required.


Installing Bind
===============

Here you will see how easy is to install a DNS server. Bellow you will see a howto for Ubuntu and Centos

Ubuntu
------

.. highlight:: bash

::

    $ apt-get install bind9 bind9utils -y


Centos
------

.. highlight:: bash

::

    $ yum install bind bind-utils -y
    $ chkconfig named on
    $ service named start


Configuring Bind
================

Forwarder
---------

First we will show how to configure your DNS as a forwarder.
Into the config file,  insert the forwarders directive inside the "options" main directive.
You can use the google's public DNS(8.8.8.8/8.8.4.4) as forwarder or your company's DNS. It should look like that:

Ubuntu
++++++

.. highlight:: bash

::

    $ egrep -v '//|^$' /etc/bind/named.conf.options
    options {
            directory "/var/cache/bind";
            forwarders {
                    8.8.8.8;
                    8.8.4.4;
            };
            dnssec-validation auto;
            auth-nxdomain no;    # conform to RFC1035
            listen-on-v6 { any; };
    };


Centos
++++++

.. highlight:: bash

::

    $   egrep -v '//|^$' /etc/named.conf |head
    options {
        forwarders { 8.8.8.8; 8.8.4.4; };
        listen-on port 53 { any; };
        listen-on-v6 port 53 { ::1; };
        directory           "/var/named";
        dump-file           "/var/named/data/cache_dump.db";
        statistics-file     "/var/named/data/named_stats.txt";
        memstatistics-file  "/var/named/data/named_mem_stats.txt";
        allow-query         { any; }";
        recursion yes;

DNS Zone
--------

Now we will set a DNS Zone to be used by tsuru. In this example we are using the domain cloud.company.com.
Create a entrance for that into /etc/bind/named.conf.local(for ubuntu) or /etc/named.conf(for centos)  as following:

Ubuntu
++++++

.. highlight:: bash

::

    zone "cloud.company.com" {
            type master;
            file "/etc/bind/db.cloud.company.com";
    };

Centos
++++++

.. highlight:: bash

::

    zone "cloud.company.com" {
            type master;
            file "db.cloud.company.com";
    };

And create a db.cloud.company.com file(considering the your external IP for tsuru, hipache and git is 192.168.123.131) the way below:

.. highlight:: bash

::

   $  cat db.cloud.company.com
   ;
   $TTL    604800
   @       IN      SOA     cloud.company.com. tsuru.cloud.company.com. (
                                 3         ; Serial
                            604800         ; Refresh
                             86400         ; Retry
                           2419200         ; Expire
                            604800 )       ; Negative Cache TTL
   ;
   @       IN      NS      cloud.company.com.
   @       IN      A       192.168.123.131
   git     IN      A       192.168.123.131 ; here we can set a better exhibition for the git remote provided by tsuru
   *       IN      A       192.168.123.131

Ps: If you have problems, it could be related with the date of your machine. We recommend you to install a ntpd service.

Now just reload your DNS server, point it to your resolv.conf, and use tsuru!
To test, just execute the command below, and see if all responses resolv to 192.168.123.131:

.. highlight:: bash

::

   $ ping cloud.company.com
   $ ping git.cloud.company.com
   $ ping zzzzz.cloud.company.com
   $ ping anydomain.cloud.company.com
