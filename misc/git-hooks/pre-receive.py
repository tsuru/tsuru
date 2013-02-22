#!/usr/bin/env python

import urllib2
import os
import sys


tsuru_host = os.environ.get("TSURU_HOST", "")
app_name = os.getcwd().split("/")[-1].replace(".git", "")
timeout = 1800

try:
    f = urllib2.urlopen("{0}/apps/{1}/avaliable".format(tsuru_host, app_name),
                        timeout=timeout)
except urllib2.HTTPError as e:
    sys.stderr.write("\n ---> {0}\n".format(e.read()))
    sys.exit(1)
