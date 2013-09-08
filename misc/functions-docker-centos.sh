TSURU_USER=tsuru
TSURU_BIN=/usr/local/bin
TSURU_LOG=/var/log/tsuru
TSURU_DOMAIN=cloud.company.com

##### Functions for Start Setup #####

function get_interface_ip() {
	ifconfig $1 | grep -oP "\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3}"|head -n1
}
EXTIP=$(get_interface_ip eth0)
INTIP=$(get_interface_ip docker0)


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
    curl https://raw.github.com/globocom/tsuru/master/misc/git-hooks/post-receive -o /home/git/bare-template/hooks/post-receive
    chmod +x /home/git/bare-template/hooks/*
    mkdir -p /home/git/.ssh /var/log/gandalf
    chown -R git:git /home/git /var/repositories /var/log/gandalf
}

function configure_git_hooks() {
    # the post-receive hook requires some environment variables to be set
    token=$(/usr/bin/tsr token)
    echo -e "export TSURU_TOKEN=$token\nexport TSURU_HOST=http://127.0.0.1:8080" |sudo -u git tee -a ~git/.bash_profile
}

function use_https_in_git() {
    # this enables npm to work properly
    # npm installs packages using the git readonly url,
    # which won't work behind a proxy
    git config --system url.https://.insteadOf git://
}


##### Functions for Tsuru Setup #####

function install_tsuru() {
    echo "Downloading tsuru binary and copying to TSURU_BIN"
    curl -sL https://s3.amazonaws.com/tsuru/dist-server/tsr.tar.gz | tar -xz -C $TSURU_BIN
}

function configure_tsuru() {
    echo "Configuring tsuru"
    mkdir /etc/tsuru
    curl -sL https://raw.github.com/globocom/tsuru/master/etc/tsuru-docker.conf -o /etc/tsuru/tsuru.conf
    #trying to configure tsuru for you
    sed -i "s|user: ubuntu|user: $TSURU_USER|; \
	 s|id_rsa.pub|id_dsa.pub|; \
	 s|/home/ubuntu/|/home/tsuru/|; \
	 s|domain: cloud.company.com|domain: $TSURU_USER|; \
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
        curl -O https://raw.github.com/globocom/tsuru/master/misc/platforms-setup.js
    fi
    mongo tsuru platforms-setup.js
}


##### Functions for Hipache Setup #####

function install_npm() {
    curl http://nodejs.org/dist/v0.8.23/node-v0.8.23-linux-x64.tar.gz | tar -C $TSURU_BIN/.. --strip-components=1 -zxv
}

function install_hipache() {
    npm install hipache -g
}

function configure_hipache() {
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
    mkdir /var/log/hipache
}


##### Functions for Circus Setup #####

function install_circus() {
	pip install circus
	mkdir /etc/circus
}

function configure_circus() {
	curl https://raw.github.com/globocom/tsuru/master/misc/circus-docker.ini -o /etc/circus/circus.ini.download
	sed "s|/usr/bin/tsr|$TSURU_BIN/tsr|; s|/var/log/tsuru|$TSURU_LOG|; s|uid = ubuntu|uid = $TSURU_USER|; s|/usr/local/bin/docker|/usr/bin/docker|; s|/usr/bin/gandalf-webserver|$TSURU_BIN/gandalf-webserver|; s|/usr/local/bin/hipache|$TSURU_BIN/hipache|; s|registry.cloud.company.com|registry.$TSURU_DOMAIN|; /.watcher:beanstalkd./,/^$/ d;" /etc/circus/circus.ini.download > etc/circus/circus.ini
	mkdir $TSURU_LOG
	chown -R $TSURU_USER:$TSURU_USER $TSURU_LOG
	rm -f /etc/circus/circus.ini.download 
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
	configure_gandalf
	configure_git_hooks
	use_https_in_git
	configure_hipache
	configure_circus
	setup tsuru
	setup_platforms
}
