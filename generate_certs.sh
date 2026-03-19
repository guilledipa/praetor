#!/bin/bash

set -e

NATS_CERT_DIR="nats/certs"
MASTER_CERT_DIR="master/certs"
AGENT_CERT_DIR="agent/certs"

mkdir -p $NATS_CERT_DIR
mkdir -p $MASTER_CERT_DIR
mkdir -p $AGENT_CERT_DIR

# Cleanup old certs
rm -f $NATS_CERT_DIR/*
rm -f $MASTER_CERT_DIR/*
rm -f $AGENT_CERT_DIR/*

echo "--- Generating Unified PKI CA ---"
openssl genrsa -out $MASTER_CERT_DIR/ca.key 4096
openssl req -x509 -new -nodes -key $MASTER_CERT_DIR/ca.key -sha256 -days 3650 -out $MASTER_CERT_DIR/ca.crt -subj "/CN=PraetorUnifiedCA"

cp $MASTER_CERT_DIR/ca.crt $NATS_CERT_DIR/ca.crt

echo "--- Generating NATS Certificates ---"
# 2. Generate NATS Server key and CSR
openssl genrsa -out $NATS_CERT_DIR/server.key 4096
openssl req -new -key $NATS_CERT_DIR/server.key -out $NATS_CERT_DIR/server.csr -subj "/CN=localhost"

# 3. Sign NATS Server certificate with Unified CA
openssl x509 -req -in $NATS_CERT_DIR/server.csr -CA $MASTER_CERT_DIR/ca.crt -CAkey $MASTER_CERT_DIR/ca.key -CAcreateserial -out $NATS_CERT_DIR/server.crt -days 3650 -sha256 -extfile <(printf "subjectAltName=DNS:localhost,IP:127.0.0.1")

# 4. Generate Default NATS Client key and CSR (for Master to use)
openssl genrsa -out $NATS_CERT_DIR/client.key 4096
openssl req -new -key $NATS_CERT_DIR/client.key -out $NATS_CERT_DIR/client.csr -subj "/CN=PraetorNATSClient"

# 5. Sign NATS Client certificate with Unified CA
openssl x509 -req -in $NATS_CERT_DIR/client.csr -CA $MASTER_CERT_DIR/ca.crt -CAkey $MASTER_CERT_DIR/ca.key -CAcreateserial -out $NATS_CERT_DIR/client.crt -days 3650 -sha256

echo "NATS Certificates generated in $NATS_CERT_DIR"

echo "--- Generating Master gRPC Certificates ---"

# 2. Generate Master Server key and CSR
openssl genrsa -out $MASTER_CERT_DIR/server.key 4096
openssl req -new -key $MASTER_CERT_DIR/server.key -out $MASTER_CERT_DIR/server.csr -subj "/CN=localhost"

# 3. Sign Master Server certificate with Unified CA
openssl x509 -req -in $MASTER_CERT_DIR/server.csr -CA $MASTER_CERT_DIR/ca.crt -CAkey $MASTER_CERT_DIR/ca.key -CAcreateserial -out $MASTER_CERT_DIR/server.crt -days 3650 -sha256 -extfile <(printf "subjectAltName=DNS:localhost,IP:127.0.0.1")

# 4. Generate Pre-provisioned Agent Client key and CSR (deprecated long-term)
openssl genrsa -out $AGENT_CERT_DIR/client.key 4096
openssl req -new -key $AGENT_CERT_DIR/client.key -out $AGENT_CERT_DIR/client.csr -subj "/CN=PraetorPreprovisionedAgent"

# 5. Sign Pre-provisioned Agent Client certificate with Unified CA
openssl x509 -req -in $AGENT_CERT_DIR/client.csr -CA $MASTER_CERT_DIR/ca.crt -CAkey $MASTER_CERT_DIR/ca.key -CAcreateserial -out $AGENT_CERT_DIR/client.crt -days 3650 -sha256

# 6. Copy Unified CA cert to Agent certs dir for initial trust
cp $MASTER_CERT_DIR/ca.crt $AGENT_CERT_DIR/master-ca.crt

# 7. Generate Master Signing Keypair (ed25519)
openssl genpkey -algorithm ed25519 -out $MASTER_CERT_DIR/master_signing.key
openssl pkey -in $MASTER_CERT_DIR/master_signing.key -pubout -out $MASTER_CERT_DIR/master_signing.pub

echo "Master Certificates generated in $MASTER_CERT_DIR"
ls -l $MASTER_CERT_DIR
echo "Agent Certificates generated in $AGENT_CERT_DIR"
ls -l $AGENT_CERT_DIR
