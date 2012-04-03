# -*- coding: utf-8 -*-
from fabric.api import cd, env, run, settings

env.user = 'ubuntu'
env.tsuru_path = '/home/ubuntu/.go/src/github.com/timeredbull/tsuru'
env.collector_path = '%s/collector' % env.tsuru_path
env.webserver_path = '%s/api/webserver' % env.tsuru_path


def stop():
    with settings(warn_only=True):
        run('killall -KILL webserver collector')


def update():
    run('go get -u github.com/timeredbull/tsuru/collector')
    run('go get -u github.com/timeredbull/tsuru/api/webserver')


def build():
    with cd(env.collector_path):
        run("go build -o collector main.go")

    with cd(env.webserver_path):
        run("go build -o webserver main.go")


def start():
    run("nohup %s/collector >& /dev/null < /dev/null &" % env.collector_path, pty=False)
    run("nohup %s/webserver >& /dev/null < /dev/null &" % env.webserver_path, pty=False)


def deploy():
    stop()
    update()
    build()
    start()
