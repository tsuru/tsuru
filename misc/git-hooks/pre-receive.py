#!/usr/bin/env python

import urllib2
import os
import sys


tsuru_host = os.environ.get("TSURU_HOST", "")
token = os.environ.get("TSURU_TOKEN", "")
owner = os.environ.get("TSURU_TOKEN_OWNER", "")
app_name = os.getcwd().split("/")[-1].replace(".git", "")
timeout = 1800

try:
    headers = {"Authorization": token, "Token-Owner": owner}
    url = "{0}/apps/{1}/avaliable".format(tsuru_host, app_name)
    request = urllib2.Request(url, headers=headers)
    f = urllib2.urlopen(request, timeout=timeout)
except urllib2.HTTPError as e:
    sys.stderr.write("\n ---> {0}\n\n".format(e.read()))
    sys.exit(1)
except:
    sys.stderr.write("\n ---> Failed to communicate with tsuru server\n")
    sys.exit(1)
