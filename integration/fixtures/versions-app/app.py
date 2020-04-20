import os
from flask import Flask
app = Flask(__name__)


@app.route('/')
def hello():
    return 'app: {} - version: {}'.format(os.environ.get("TSURU_APPNAME"), os.environ.get("TSURU_APPVERSION"))


if __name__ == '__main__':
    app.run(host="0.0.0.0", port=int(os.environ.get("PORT", 8888)))
