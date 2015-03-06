.. Copyright 2015 tsuru authors. All rights reserved.
   Use of this source code is governed by a BSD-style
   license that can be found in the LICENSE file.

Roadmap
-------

Release Process
===============

We use GitHub's milestones to releases' planning and anyone is free to
suggest an issue to a milestone, and discuss about any issue in the next tsuru
version. We also have internal goals as listed bellow and our focus will be
these goals. But it's not immutable, we can change any goal at any time as
community need.

At globo.com we have goals by quarter of a year (short term goals bellow), but
it doesn't mean that there's only one release per quarter. Our releases have
one or more main issues and minor issues which can be minor bugfixes, ground
work issue and other "not so important but needed" issues.

You can suggest any issue to any milestones at any time, and we'll
discuss it in the issue or in `gitter <gitter.im/tsuru/tsuru>`_.

Next Release 0.11.0 (until July)
================================

These goals are defined by quarter at globo.com but it can change as community
need.

    - auto scale machine pool (issue `#1110 <https://github.com/tsuru/tsuru/issues/1110>`_).

      We need to auto-scale machine pool to be more resilient.

    - `unit-remove` use segreggate schedule (issue `#1109 <https://github.com/tsuru/tsuru/issues/1109>`_).

      Now, the unit-remove command is random and it can create an unbalanced
      pool, so we have to use segregate schedule to remove units more safely.

    - dockerize tsuru installation (issue `#1091 <https://github.com/tsuru/tsuru/issues/1091>`_).

      We'll run all tsuru components in containers, so our installation will be
      more clean, and fast, and easy.


Long term Goals
===============

These goals are our goals to 1.0 version.

    - review platform management.

      We are thinking to change our way to manage platform. Today tsuru has your own platform. But we have a lot of problem to mantain it.
      In other way, we have buildpacks and we can use it to provide any platform we want, but there's no free lunch.
      Buildpack can be build by any one and are updated automagicly, so your application can stop work and you'll can do anything to fix it.

    - docker images with envs.

    - get logs and metrics from outside of app container.

    - improve `app-swap`

    - improve plugins
