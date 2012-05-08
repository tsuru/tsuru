#Tsuru

##What is Tsuru?

Tsuru is an open Platform-as-a-Service (PaaS).

##Dependencies

Tsuru depends on [Go](http://golang.org) and [libyaml](http://pyyaml.org/wiki/LibYAML).

To install Go, follow the official instructions in the language website:
http://golang.org/doc/install.

To install libyaml, you can use one package manager, or download it and install
from source. To install from source, follow the instructions on PyYAML wiki:
http://pyyaml.org/wiki/LibYAML.

The following instructions are system specific:

###FreeBSD

    % cd /usr/ports/textproc/libyaml
    % make install clean

###Mac OS X (homebrew)

    % brew install libyaml

###Ubuntu

    % [sudo] apt-get install libyaml-dev

###CentOS

    % [sudo] yum install libyaml-devel

##Installation

After install and configure go, and install libyaml, just run in your terminal:

    % go get github.com/timeredbull/tsuru/...
