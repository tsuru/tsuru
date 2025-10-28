#!/bin/bash
# Script to generate a unique hash for this deployment
echo "$(date +%s)-$(uuidgen | cut -c1-8)" | md5sum | cut -c1-16 > version_hash.txt
echo "Generated hash: $(cat version_hash.txt)"