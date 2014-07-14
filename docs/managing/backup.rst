.. Copyright 2014 tsuru authors. All rights reserved.
   Use of this source code is governed by a BSD-style
   license that can be found in the LICENSE file.

+++++++++++++++++++++++++
Backing up tsuru database
+++++++++++++++++++++++++

In the tsuru repository, you will find two useful scripts in the directory
``misc/mongodb``: ``backup.bash`` and ``healer.bash``. In this page you will
learn the purpose of these scripts and how to use them.

Dependencies
++++++++++++

The script ``backup.bash`` uses S3 to store archives, and ``healer.bash``
downloads archives from S3 buckets. In order to communicate with S3 API, both
scripts use `s3cmd <http://s3tools.org/s3cmd>`_.

So, before running those scripts, make sure you have installed s3cmd. You can
install it using your preferred package manager. For more details, refer to its
`download documentation <http://s3tools.org/download>`_.

After installing s3cmd, you will need to configure it, by running the command:

.. highlight:: bash

::

    $ s3cmd --configure

Saving data
+++++++++++

The script ``backup.bash`` runs ``mongodump``, creates a tar archive and send
the archive to S3. Here is how you use it:

.. highlight:: bash

::

    $ ./misc/mongodb/backup.bash s3://mybucket localhost database

The first parameter is the S3 bucket. The second parameter is the database
host. You can provide just the hostname, or the host:port (for example,
127.0.0.1:27018). The third parameter is the name of the database.

Automatically restoring on data loss
++++++++++++++++++++++++++++++++++++

The other script in the ``misc/mongodb`` directory is ``healer.bash``. This
script checks a list of collections and if any of them is gone, download the
last three backup archives and fix all gone collections.

This is how you should use it:

.. highlight:: bash

::

    $ ./misc/mongodb/healer.bash s3://mybucket localhost mongodb repositories users

The first three parameters mean the same as in the backup script. From the
fourth parameter onwards, you should list the collections. In the example
above, we provided two collections: "repositories" and "users".
