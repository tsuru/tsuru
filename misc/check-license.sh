#!/bin/sh -e

for f in `grep "Copyright 2012" -r . -l`
do
	date=`git log -1 --format="%ad" --date=short -- $f`
	if [ `echo "$date" | grep ^2013` ]
	then
		echo $f $date
	fi
done
