#!/bin/bash

set -e

if [ -w /home/git/.ssh ]; then
    chown git:git /home/git/.ssh && chmod 700 /home/git/.ssh
fi

if [ ! -e /home/git/.ssh/authorized_keys ]; then
    touch /home/git/.ssh/authorized_keys
fi
if [ -w /home/git/.ssh/authorized_keys ]; then
    chown git:git /home/git/.ssh/authorized_keys
    chmod 600 /home/git/.ssh/authorized_keys
fi
if [ ! -e /home/git/.ssh/environment ]; then
    echo "MONGODB_ADDR=$MONGODB_ADDR" > /home/git/.ssh/environment
    echo "MONGODB_PORT=$MONGODB_PORT" >> /home/git/.ssh/environment
    echo "TSURU_HOST=$TSURU_HOST" >> /home/git/.ssh/environment
    echo "TSURU_TOKEN=$TSURU_TOKEN" >> /home/git/.ssh/environment
fi

echo "Starting sshd"
/usr/sbin/sshd
echo "Starting rsyslogd"
/usr/sbin/rsyslogd
echo "Running gandalf-server"
exec su git -c "/bin/gandalf-webserver"
