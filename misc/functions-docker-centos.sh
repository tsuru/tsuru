TSURU_USER=tsuru
TSURU_BIN=/usr/local/bin
TSURU_LOG=/var/log/tsuru
TSURU_DOMAIN=cloud.company.com

#If you let the environment bellow unset, we will try to identify your correct IP. Docker must be running in order to detect its interface
#IP Accessible via a client machine
EXTIP=
#Docker IP
INTIP=

##### Functions for Start Setup #####

function get_interface_ip() {
	ifconfig $1 | grep -oP "\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3}"|head -n1
}
test -z "$EXTIP" && EXTIP=$(get_interface_ip eth0)
test -z "$INTIP" && INTIP=$(get_interface_ip docker0)


##### Functions for Gandalf Setup #####

function install_gandalf() {
    echo "Installing gandalf-wrapper"
    curl -sL https://s3.amazonaws.com/tsuru/dist-server/gandalf-bin.tar.gz | tar -xz -C $TSURU_BIN
    echo "Installing gandalf-webserver"
    curl -sL https://s3.amazonaws.com/tsuru/dist-server/gandalf-webserver.tar.gz | tar -xz -C $TSURU_BIN
}

function configure_gandalf() {
    echo "Configuring gandalf"
    echo "bin-path: $TSURU_BIN/gandalf-bin
database:
  url: 127.0.0.1:27017
  name: gandalf
git:
  bare:
    location: /var/repositories
    template: /home/git/bare-template
  daemon:
    export-all: true
host: $TSURU_DOMAIN
webserver:
  port: \":8000\"" > /etc/gandalf.conf

    echo "Creating git user"
    grep ^git: /etc/passwd > /dev/null 2>&1 || useradd -m git
    echo "Creating bare path"
    [ -d /var/repositories ] || mkdir -p /var/repositories
    echo "Creating template path"
    [ -d /home/git/bare-template/hooks ] || mkdir -p /home/git/bare-template/hooks
    curl https://raw.github.com/tsuru/tsuru/master/misc/git-hooks/post-receive -o /home/git/bare-template/hooks/post-receive
    chmod +x /home/git/bare-template/hooks/*
    mkdir -p /home/git/.ssh /var/log/gandalf
    chown -R git:git /home/git /var/repositories /var/log/gandalf
}

function configure_git_hooks() {
    echo "Configuring git_hooks"
    # the post-receive hook requires some environment variables to be set
    token=$($TSURU_BIN/tsr token)
    echo -e "export TSURU_TOKEN=$token\nexport TSURU_HOST=http://127.0.0.1:8080" |sudo -u git tee -a ~git/.bash_profile
}

function use_https_in_git() {
    echo "Configuring git_with_npm"
    # this enables npm to work properly
    # npm installs packages using the git readonly url,
    # which won't work behind a proxy
    git config --system url.https://.insteadOf git://
}


##### Functions for Tsuru Setup #####

function install_tsuru() {
    echo "Downloading tsuru binary and copying to TSURU_BIN"
    curl -sL https://s3.amazonaws.com/tsuru/dist-server/tsr-master.tar.gz | tar -xz -C $TSURU_BIN
}

function configure_tsuru() {
    echo "Configuring tsuru"
    mkdir -p /etc/tsuru
    curl -sL https://raw.github.com/tsuru/tsuru/master/etc/tsuru-docker.conf -o /etc/tsuru/tsuru.conf
    #trying to configure tsuru for you
    sed -i "s|^host:.*|host: $EXTIP:8080|; \
	 s|id_rsa.pub|id_dsa.pub|; \
	 s|/home/ubuntu/|/home/tsuru/|; \
	 s|domain: cloud.company.com|domain: $TSURU_DOMAIN|; \
	 s|http://localhost:4243|http://$INTIP:4243|; \
	 s|rw-host: my.gandalf.domain|rw-host: $EXTIP|; \
	 s|ro-host: 172.16.42.1|ro-host: $INTIP|" /etc/tsuru/tsuru.conf

    # make sure the tsuru user exists
    if ! id $TSURU_USER > /dev/null 2>&1
    then
        useradd -m $TSURU_USER
        sudo -u $TSURU_USER mkdir -p /home/$TSURU_USER/.ssh
    fi
    sudo -u $TSURU_USER ssh-keygen -t dsa -f /home/$TSURU_USER/.ssh/id_dsa -N ""
}

function setup_platforms() {
    # this function should be called in the provisioner specific installation script
    # because mongo usually takes some time to startup, and it's not safe to call it from here
    # so call it after everything runs
    if [ ! -f platforms-setup.js ]; then
        curl -O https://raw.github.com/tsuru/tsuru/master/misc/platforms-setup.js
    fi
    mongo tsuru platforms-setup.js
}


##### Functions for Hipache Setup #####

function install_npm() {
    echo "Downloading NPM binary and copying to TSURU_BIN"
    curl http://nodejs.org/dist/v0.8.23/node-v0.8.23-linux-x64.tar.gz | tar -C $TSURU_BIN/.. --strip-components=1 -zx
}

function install_hipache() {
    echo "Installing hipache"
    npm install hipache -g
}

function configure_hipache() {
    echo "Configuring hipache"
    echo "{
    \"server\": {
        \"accessLog\": \"/var/log/hipache/hipache_access.log\",
        \"port\": 80,
        \"workers\": 5,
        \"maxSockets\": 100,
        \"deadBackendTTL\": 30
    },
    \"redisHost\": \"127.0.0.1\",
    \"redisPort\": 6379
}" > /etc/hipache.conf.json
    mkdir -p /var/log/hipache
}


##### Functions for Circus Setup #####

function install_circus() {
    echo "Installing hipache"
    pip install circus
    mkdir -p /etc/circus
}

function configure_circus() {
    echo "Configuring circus"
    curl https://raw.github.com/tsuru/tsuru/master/misc/circus-docker.ini -o /etc/circus/circus.ini.download
    sed "s|/usr/bin/tsr|$TSURU_BIN/tsr|; s|/var/log/tsuru|$TSURU_LOG|; s|uid = ubuntu|uid = $TSURU_USER|; s|/usr/local/bin/docker|/usr/bin/docker|; s|/usr/bin/gandalf-webserver|$TSURU_BIN/gandalf-webserver|; s|/usr/local/bin/hipache|$TSURU_BIN/hipache|; s|registry.cloud.company.com|registry.$TSURU_DOMAIN|; /.watcher:beanstalkd./,/^$/ d;" /etc/circus/circus.ini.download > /etc/circus/circus.ini
    mkdir -p /var/log/docker
    chown -R $TSURU_USER:$TSURU_USER $TSURU_LOG
    rm -f /etc/circus/circus.ini.download 
    echo -e 'description\t"Run circusd"\n\nstart on filesystem or runlevel [2345]\nstop on runlevel [!2345]\n\nrespawn\n\nscript\n/usr/bin/circusd --log-output /var/log/circus.log --pidfile /var/run/circusd.pid /etc/circus/circus.ini\nend script' > /etc/init/circusd.conf
}


##### Main

#It can be used to upgrade all the services as you want. No configuration will be made
function install_services() {
	install_gandalf
	install_tsuru
	install_npm
	install_hipache
	install_circus
}

#It is supposed to be executed just once. Take care with it!
function configure_services_for_first_time() {
	configure_tsuru
	configure_gandalf
	configure_git_hooks
	use_https_in_git
	configure_hipache
	configure_circus
	setup_platforms
}
