# -*- coding: utf-8 -*-

# Copyright 2013 tsuru authors. All rights reserved.
# Use of this source code is governed by a BSD-style
# license that can be found in the LICENSE file.

import os
from fabric.api import abort, cd, env, local, put, run, sudo

current_dir = os.path.abspath(os.path.dirname(__file__))
env.tsuru_path = '/home/%s/tsuru' % env.user


def build(flags="", tags=""):
    if tags != "":
        flags += " -tags '%s'" % tags
    goos = local("go env GOOS", capture=True)
    goarch = local("go env GOARCH", capture=True)
    if goos != "linux" or goarch != "amd64":
        abort("tsuru must be built on linux_amd64 for deployment, " +
              "you're on %s_%s" % (goos, goarch))
    local("mkdir -p dist")
    local("go clean ./...")
    local("go build %s -a -o dist/tsr ./cmd/tsr" % flags)


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
    run('circusctl restart web')
    run('circusctl restart collector')


def deploy(flags="", tags=""):
    build(flags, tags)
    send()
    restart()
    clean()


def deploy_hooks(path, template_path, user="git", group="git"):
    run("mkdir -p /tmp/git-hooks")
    put("misc/git-hooks/*", "/tmp/git-hooks")
    sudo("chown -R %s:%s /tmp/git-hooks" % (user, group))
    sudo("chmod 755 /tmp/git-hooks/*")
    out = run("find %s -name \*.git -type d" % path)
    paths = [p.strip() for p in out.split("\n")]
    for path in paths:
        sudo("cp -p /tmp/git-hooks/* %s/hooks" % path)
    sudo("cp -p /tmp/git-hooks/* %s/hooks" % template_path)
    sudo("rm /tmp/git-hooks/*")
    sudo("rmdir /tmp/git-hooks")
