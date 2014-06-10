#!/bin/bash -el

while read oldrev newrev refname
do
        COMMIT=${newrev}
done

AUTH_URL=https://yourswift.com/auth/v1.0
AUTH_PARAMS="-K yourkey -U youruser"

APP_DIR=${PWD##*/}
APP_NAME=${APP_DIR/.git/}
CONTAINER_NAME=yourbucket
UUID=`python -c 'import uuid; print uuid.uuid4().hex'`
ARCHIVE_FILE_NAME=${APP_NAME}_${COMMIT}_${UUID}.tar.gz
git archive --format=tar.gz -o /tmp/$ARCHIVE_FILE_NAME $COMMIT
swift -qA $AUTH_URL $AUTH_PARAMS upload $CONTAINER_NAME /tmp/$ARCHIVE_FILE_NAME --object-name $ARCHIVE_FILE_NAME
swift -qA $AUTH_URL $AUTH_PARAMS post -r ".r:*" $CONTANER_NAME
rm /tmp/$ARCHIVE_FILE_NAME
ARCHIVE_URL=`swift -A $AUTH_URL $AUTH_PARAMS stat -v $CONTAINER_NAME $ARCHIVE_FILE_NAME | grep URL | awk -F': ' '{print $2}'`
URL="${TSURU_HOST}/apps/${APP_NAME}/repository/clone"
curl -H "Authorization: bearer ${TSURU_TOKEN}" -d "archive-url=${ARCHIVE_URL}&commit=${COMMIT}" -s -N --max-time 1800 $URL
swift -qA $AUTH_URL $AUTH_PARAMS delete $CONTAINER_NAME $ARCHIVE_FILE_NAME
