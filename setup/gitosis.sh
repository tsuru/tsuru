#!/bin/bash -e

BARE_PATH=/mnt/repositories
REPO_PATH=/mnt/gitosis-admin
GITOSIS_CONF=/etc/gitosis/gitosis.conf
KEY_NAME=ubuntu@tsuru.pub
ORIGINAL_KEY_PATH=/home/ubuntu/.ssh/id_rsa.pub
KEY_PATH=/home/git/${KEY_NAME}
CONF_CONTENT="
[gitosis]
repositories = ${BARE_PATH}
generate-files-in = /mnt/gitosis
gitweb = no
daemon = no
"
CONF_CONTENT_ESCAPED='[gitosis]\
repositories = /mnt/repositories\
generate-files-in = /mnt/gitosis\
gitweb = no\
daemon = no'

grep -qFx "export TSURU_HOST=http://localhost:8080" /etc/profile
if  [ $? -ne 0 ]
then
    echo "Adding TSURU_HOST to /etc/profile..."
    sudo /bin/bash -c 'echo "export TSURU_HOST=http://localhost:8080" >> /etc/profile'
fi
source /etc/profile

echo "Copying ubuntu's public key to git home. Will generate one it doesn't exists..."
if [ ! -f $KEY_PATH ]
then
    ssh-keygen -t rsa -f /home/ubuntu/.ssh/id_rsa -P ""
fi
sudo cp ${ORIGINAL_KEY_PATH} ${KEY_PATH}
sudo chown git:git ${KEY_PATH}

echo "Creating gitosis.conf in /etc/gitosis/..."
[ -d /etc/gitosis ] && echo "gitosis.conf already exists" || sudo mkdir /etc/gitosis
sudo chown git:git /etc/gitosis -R
[ -f ${GITOSIS_CONF} ] || sudo -u git /bin/bash -c "echo \"${CONF_CONTENT}\" >> ${GITOSIS_CONF}"

echo "Initializing gitosis..."
sudo -u git /bin/bash -c "gitosis-init --config=${GITOSIS_CONF} < ${KEY_PATH}"
sudo chown git:git ${BARE_PATH} -R
sudo chmod g+rw ${BARE_PATH} -R
rm -rf ${BARE_PATH}/gitosis-admin.git/hooks/post-receive # this hook is only for tsuru's apps repositories
ln -fs ${BARE_PATH}/gitosis-admin.git/gitosis.conf /home/ubuntu/.gitosis.conf

echo "Cloning gitosis-admin.git to ${REPO_PATH}..."
sudo mkdir ${REPO_PATH}
sudo chown git:git ${REPO_PATH}
sudo chmod g+rw ${REPO_PATH}
git clone ${BARE_PATH}/gitosis-admin.git ${REPO_PATH}
sudo chown git:git ${REPO_PATH} -R
sudo chmod g+rw ${REPO_PATH} -R

echo "Editing gitosis.conf in ${REPO_PATH} to match ${GITOSIS_CONF}..."
sed -ie "s,\[gitosis\],$CONF_CONTENT_ESCAPED," ${REPO_PATH}/gitosis.conf
git --git-dir=${REPO_PATH}/.git --work-tree=${REPO_PATH} add .
git --git-dir=${REPO_PATH}/.git --work-tree=${REPO_PATH} commit -m "Adding default gitosis conf"
git --git-dir=${REPO_PATH}/.git --work-tree=${REPO_PATH} push origin master
git --git-dir=${REPO_PATH}/.git --work-tree=${REPO_PATH} remote add origin2 git@localhost:gitosis-admin.git
git --git-dir=${REPO_PATH}/.git --work-tree=${REPO_PATH} remote rm origin
git --git-dir=${REPO_PATH}/.git --work-tree=${REPO_PATH} remote rename origin2 origin

sudo chown git:git ${BARE_PATH} -R
sudo chmod g+rw ${BARE_PATH} -R

echo "Starting git daemon..."
git daemon --base-path=${BARE_PATH} --export-all --syslog &
