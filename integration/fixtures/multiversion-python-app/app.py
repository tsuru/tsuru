import os
from flask import Flask, jsonify

app = Flask(__name__)

def read_version_hash():
    try:
        with open('version_hash.txt', 'r') as f:
            return f.read().strip()
    except FileNotFoundError:
        return 'no-hash'

@app.route('/')
def hello():
    app_name = os.environ.get("TSURU_APPNAME", "unknown")
    app_version = os.environ.get("TSURU_APPVERSION", "unknown")
    version_hash = read_version_hash()
    return f'app: {app_name} - version: {app_version} - hash: {version_hash}'

@app.route('/health')
def health():
    return 'OK'

@app.route('/version')
def version():
    return jsonify({
        'app': os.environ.get("TSURU_APPNAME", "unknown"),
        'version': os.environ.get("TSURU_APPVERSION", "unknown"),
        'hash': read_version_hash()
    })

if __name__ == '__main__':
    app.run(host="0.0.0.0", port=int(os.environ.get("PORT", 8888)))