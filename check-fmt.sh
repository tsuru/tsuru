#!/bin/bash -e

out=`gofmt -l .`
if [ "${out}" = "" ]
then
    exit 0
else
    echo "ERROR: there are files that need to be formatted with gofmt"
    echo
    echo "Files:"
    for file in $out
    do
        echo "- ${file}"
    done
    exit 1
fi
