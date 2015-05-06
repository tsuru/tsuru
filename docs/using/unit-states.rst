.. Copyright 2015 tsuru authors. All rights reserved.
   Use of this source code is governed by a BSD-style
   license that can be found in the LICENSE file.

Unit states
===========

Unit status a the way to know what is happening with an unit. You can use the
`tsuru app-info -a <appname>` to see the unit status:

.. highlight:: bash

::

    $ tsuru app-info -a tsuru-dashboard
    Application: tsuru-dashboard
    Repository: git@localhost:tsuru-dashboard.git
    Platform: python
    ...
    Units: 1
    +------------+---------+
    | Unit       | State   |
    +------------+---------+
    | 9cf863c2c1 | started |
    +------------+---------+

The unit state flow is:

.. highlight:: bash

::

    +----------+                           start          +---------+
    | building |                   +---------------------+| stopped |
    +----------+                   |                      +---------+
          ^                        |                           ^
          |                        |                           |
     deploy unit                   |                         stop
          |                        |                           |
          +                        v       RegisterUnit        +
     +---------+  app unit   +----------+  SetUnitStatus  +---------+
     | created | +---------> | starting | +-------------> | started |
     +---------+             +----------+                 +---------+
                                   +                         ^ +
                                   |                         | |
                             SetUnitStatus                   | |
                                   |                         | |
                                   v                         | |
                               +-------+     SetUnitStatus   | |
                               | error | +-------------------+ |
                               +-------+ <---------------------+

* `created`: is the initial status of an unit.
* `building`: is the status for units being provisioned by the provisioner, like in the deployment.
* `error`: is the status for units that failed to start, because of an application error.
* `starting`: is set when the container is started in docker.
* `started`: is for cases where the unit is up and running.
* `stopped`: is for cases where the unit has been stopped.
