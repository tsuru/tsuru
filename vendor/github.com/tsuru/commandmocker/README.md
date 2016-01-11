#commandmocker

[![Build Status](https://secure.travis-ci.org/tsuru/commandmocker.png?branch=master)](http://travis-ci.org/tsuru/commandmocker)

commandmocker is a simple utility for tests in Go. It adds command with expected output to the path.

For example, if you want to mock the command "ssh", you can write a test that looks like this:

    import (
        "github.com/tsuru/commandmocker"
        "testing"
    )

    func TestScreamsIfSSHFail(t *testing.T) {
        message := "ssh: Could not resolve hostname myhost: nodename nor servname provided, or not known"
        path, err := commandmocker.Error("ssh", message, 65)
        if err != nil {
            t.Fatal(err)
        }
        defer commandmocker.Remove(path)

        // write your test and expectations
    }
