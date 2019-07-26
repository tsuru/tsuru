#/bin/bash

set -e

go mod verify
go mod vendor

if ! git diff --quiet; then
    echo
    echo "************************************************"
    echo "ERROR! ./vendor directory does not match go.mod:"
    echo "************************************************"
    echo
    git status
    echo "Diff:"
    git diff
    exit 1
fi
