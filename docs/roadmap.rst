.. Copyright 2015 tsuru authors. All rights reserved.
   Use of this source code is governed by a BSD-style
   license that can be found in the LICENSE file.

Roadmap
-------

Release Process
===============

We use GitHub's milestones to releases' planning and anyone is free to
suggest an issue to a milestone, and discuss about any issue in the next tsuru
release. We also have internal goals as listed bellow and our focus will be on
these goals. But they are not immutable, we can change any goal at any time as
community need.

At globo.com, we have goals by quarter of a year (short term goals bellow), but
it doesn't mean that there's only one release per quarter. Our releases have
one or more main issues and minor issues which can be minor bugfixes, ground
work issue and other "not so important but needed" issues.

You can suggest any issue to any milestones at any time, and we'll
discuss it in the issue or in `Gitter <https://gitter.im/tsuru/tsuru>`_.

Next Release (0.14.0/1.0-wannabe)
=================================

    - Improve authorization system (issue `#1220 <https://github.com/tsuru/tsuru/issues/1220>`_)

      The goal is to change the authorization system to be more flexible based on `users`, `permissions` and `roles`.
      An user can have more than one role, and a role can have one or more permissions.

Long term Goals
===============

These are our goals to 1.0 version.

    - review platform management.

      We are thinking to change our way to manage platform. Today tsuru has its own platform. But we have a lot of problems to mantain it.
      In other way, we have buildpacks and we can use it to provide any platform we want, but there's no free lunch.`
      Buildpack can be built by anyone and are updated "automagically", so your application may stop to deploy properly, totally out of the blue.
