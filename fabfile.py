# -*- coding: utf-8 -*-

# Copyright 2012 tsuru authors. All rights reserved.
# Use of this source code is governed by a BSD-style
# license that can be found in the LICENSE file.

import os
from fabric.api import abort, cd, env, local, put, run

current_dir = os.path.abspath(os.path.dirname(__file__))
env.user = 'ubuntu'
env.tsuru_path = '/home/%s/tsuru' % env.user


def build(flags=""):
    goos = local("go env GOOS", capture=True)
    goarch = local("go env GOARCH", capture=True)
    if goos != "linux" or goarch != "amd64":
        abort("tsuru must be built on linux_amd64 for deployment, " +
              "you're on %s_%s" % (goos, goarch))
    local("mkdir -p dist")
    local("go clean ./...")
    local("go build %s -a -o dist/collector ./collector" % flags)
    local("go build %s -a -o dist/webserver ./api" % flags)


def clean():
    local("rm -rf dist")
    local("rm -f dist.tar.gz")


def send():
    local("tar -czf dist.tar.gz dist")
    run("mkdir -p %(tsuru_path)s" % env)
    put(os.path.join(current_dir, "dist.tar.gz"), env.tsuru_path)


def restart():
    with cd(env.tsuru_path):
        run("tar -xzf dist.tar.gz")
    run('GORACE="log_path=%s/webserver.race.log" circusctl restart web' %
        env.tsuru_path)
    run('GORACE="log_path=%s/collector.race.log" circusctl restart collector' %
        env.tsuru_path)


def deploy(flags=""):
    build(flags)
    send()
    restart()
    clean()
