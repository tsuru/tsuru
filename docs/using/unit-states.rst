unit states
===========

pending
----------

Is when the unit is waiting to be provisioned by the tsuru provisioner.

bulding
-----------

Is while the unit is provisioned, it's occurs while a deploy.

error
------

Is  when the an error occurs caused by the application code.

down
-------

Is when an error occurs caused by tsuru internal problems.

unreachable
-----------------

Is when the app process is up but it is not binded in the right host ("0.0.0.0") and right port ($PORT). If your process is a worker it's state will be `unreachable`.

started
---------

Is when the app process is up binded  in the right host ("0.0.0.0") and right port ($PORT).
