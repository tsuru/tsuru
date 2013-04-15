#!/bin/sh

if [ -f app.conf ]
then
	echo hooks: > app.yaml
	sed -e 's/^/  /' app.conf >> app.yaml
	git rm -q app.conf
	git add app.yaml
	echo "File app.conf successfully renamed to app.yaml."
	echo "Please commit the change and deploy it."
fi
