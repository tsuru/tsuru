import os
import sys
from flask import Flask

app = Flask(__name__)

@app.route('/')
def hello():
    return 'app: {} - version: {} - status: healthy'.format(
        os.environ.get("TSURU_APPNAME"), 
        os.environ.get("TSURU_APPVERSION")
    )

@app.route('/health')
def health():
    return 'OK'

@app.route('/fail')
def fail():
    # This endpoint can be used to simulate app failure
    sys.exit(1)

if __name__ == '__main__':
    app.run(host="0.0.0.0", port=int(os.environ.get("PORT", 8888)))