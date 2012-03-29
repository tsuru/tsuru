#! /bin/sh

### BEGIN INIT INFO
# Provides:          webserverd
# Required-Start:    $all
# Required-Stop:     $all
# Default-Start:     2 3 4 5
# Default-Stop:      0 1 6
# Short-Description: starts the tsuru webserver
# Description:       starts tsuru using start-stop-daemon
### END INIT INFO

PATH=/home/ubuntu/.go/src/github.com/timeredbull/tsuru/api/webserverd
DAEMON=/home/ubuntu/.go/src/github.com/timeredbull/tsuru//api/webserverd
NAME=webserverd
DESC=webserverd

test -x $DAEMON || exit 0

set -e

case "$1" in
    start)
        echo -n "Starting $DESC: "
        start-stop-daemon --start --quiet --pidfile /home/ubuntu/$NAME.pid \
            --exec $DAEMON -- $DAEMON_OPTS
        echo "$NAME."
        ;;
    stop)
        echo -n "Stopping $DESC: "
        start-stop-daemon --stop --quiet --pidfile /home/ubuntu/$NAME.pid \
            --exec $DAEMON
        echo "$NAME."
        ;;
    restart|force-reload)
        echo -n "Restarting $DESC: "
        start-stop-daemon --stop --quiet --pidfile \
            /home/ubuntu/$NAME.pid --exec $DAEMON
        sleep 1
        start-stop-daemon --start --quiet --pidfile \
            /home/ubuntu/$NAME.pid --exec $DAEMON -- $DAEMON_OPTS
        echo "$NAME."
        ;;
    reload)
        echo -n "Reloading $DESC configuration: "
        start-stop-daemon --stop --signal HUP --quiet --pidfile     /home/ubuntu/$NAME.pid \
            --exec $DAEMON
        echo "$NAME."
        ;;
    *)
        N=/etc/init.d/$NAME
        echo "Usage: $N {start|stop|restart|reload|force-reload}" >&2
        exit 1
        ;;
esac

exit 0
