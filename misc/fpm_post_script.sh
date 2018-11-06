#!/bin/sh

create_tsuru_user() {
  user="tsuru"
  exists=true
  getent passwd $user > /dev/null 2>&1 || exists=false
  if ! $exists; then
      echo "Creating user \"$user\" within group \"$user\""...
      useradd --system -md /var/lib/$user $user
  fi
}

create_tsuru_user

if [ -f /etc/default/tsurud ]; then
  . /etc/default/tsurud
  if [ "$DISABLE_PLEASERUN" == "1" ]; then
    exit 0
  fi
fi
