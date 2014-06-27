#!/bin/bash -el

# This script generates a git archive from the provided commit, uploads it to
# Swift, sends the URL to Tsuru and then delete the archive in from the
# container.
#
# It depends on the "swift" command line (it can be installed with pip).
#
# It also depends on the following environment variables:
#
#   - AUTH_PARAMS: the parameters used in authentication (for example:
#                    "-A https://yourswift.com -K yourkey -U youruser")
#   - CDN_URL: the URL of the CDN that serves content from your container (for
#              example: something.cf5.rackcdn.com).
#   - CONTAINER_NAME: name of the container where the script will store the
#                     archives
#   - TSURU_HOST: URL to the Tsuru API (for example: http://yourtsuru:8080)
#   - TSURU_TOKEN: the token to communicate with the API (generated with `tsr
#                  token`, in the server).

while read oldrev newrev refname
do
        COMMIT=${newrev}
done

APP_DIR=${PWD##*/}
APP_NAME=${APP_DIR/.git/}
UUID=`python -c 'import uuid; print uuid.uuid4().hex'`
ARCHIVE_FILE_NAME=${APP_NAME}_${COMMIT}_${UUID}.tar.gz
git archive --format=tar.gz -o /tmp/$ARCHIVE_FILE_NAME $COMMIT
swift -q $AUTH_PARAMS upload $CONTAINER_NAME /tmp/$ARCHIVE_FILE_NAME --object-name $ARCHIVE_FILE_NAME
swift -q $AUTH_PARAMS post -r ".r:*" $CONTAINER_NAME
rm /tmp/$ARCHIVE_FILE_NAME
if [ -z "${CDN_URL}" ]
then
	ARCHIVE_URL=`swift $AUTH_PARAMS stat -v $CONTAINER_NAME $ARCHIVE_FILE_NAME | grep URL | awk -F': ' '{print $2}'`
else
	ARCHIVE_URL=${CDN_URL}/${ARCHIVE_FILE_NAME}
fi
URL="${TSURU_HOST}/apps/${APP_NAME}/repository/clone"
curl -H "Authorization: bearer ${TSURU_TOKEN}" -d "archive-url=${ARCHIVE_URL}&commit=${COMMIT}" -s -N --max-time 1800 $URL | tee /tmp/deploy-${APP_NAME}.log
swift -q$AUTH_PARAMS delete $CONTAINER_NAME $ARCHIVE_FILE_NAME
tail -1 /tmp/deploy-${APP_NAME}.log | grep -q "^OK$"
