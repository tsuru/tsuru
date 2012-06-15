# -*- coding: utf-8 -*-
import os
from fabric.api import abort, cd, env, local, put, run, settings

current_dir = os.path.abspath(os.path.dirname(__file__))
env.user = 'ubuntu'
env.tsuru_path = '/home/%s/tsuru' % env.user


def stop():
    with settings(warn_only=True):
        run('killall -KILL webserver collector')


def build():
    goos = local("go env GOOS", capture=True)
    goarch = local("go env GOARCH", capture=True)
    if goos != "linux" or goarch != "amd64":
        abort("tsuru must be built on linux_amd64 for deployment, you're on %s_%s" % (goos, goarch))
    local("mkdir -p dist")
    local("go clean ./...")
    local("go build -a -o dist/collector collector/main.go")
    local("go build -a -o dist/webserver api/webserver/main.go")


def clean():
    local("rm -rf dist")
    local("rm -f dist.tar.gz")


def send():
    local("tar -czf dist.tar.gz dist")
    run("mkdir -p %(tsuru_path)s" % env)
    put(os.path.join(current_dir, "dist.tar.gz"), env.tsuru_path)


def start():
    with cd(env.tsuru_path):
        run("tar -xzf dist.tar.gz")
    run("nohup %s/dist/collector >& /tmp/collector.out < /tmp/collector.out &" % env.tsuru_path, pty=False)
    run("nohup %s/dist/webserver >& /tmp/webserver.out < /tmp/webserver.out &" % env.tsuru_path, pty=False)


def deploy():
    build()
    send()
    stop()
    start()
    clean()
