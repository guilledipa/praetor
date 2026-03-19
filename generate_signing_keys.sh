#!/bin/bash

set -e

CERT_DIR="nats/certs"

# Generate ed25519 key pair
openssl genpkey -algorithm ed25519 -outform PEM -out $CERT_DIR/master.key
openssl pkey -in $CERT_DIR/master.key -pubout -out $CERT_DIR/master.pub

echo "Signing keys generated in $CERT_DIR"
ls -l $CERT_DIR/master.*
