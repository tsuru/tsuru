#!/bin/bash -e

status=0
out=`gofmt -l .`
if [ "${out}" != "" ]
then
    echo "ERROR: there are files that need to be formatted with gofmt"
    echo
    echo "Files:"
    for file in $out
    do
        echo "- ${file}"
    done
    status=1
fi

`go vet ./... > .vet 2>&1`
out=`cat .vet`
if [ "${out}" != "" ]
then
    echo "ERROR: go vet failures:"
    echo
    cat <<END
${out}
END
    status=1
fi

rm .vet || /bin/true
exit $status
