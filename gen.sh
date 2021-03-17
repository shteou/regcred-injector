#!/bin/bash

set -o errexit
set -o nounset
set -o pipefail

SERVER_NAME="regcred-injector.admission.svc:8443"

openssl genrsa -out certs/ca.key 2048
openssl req -new -x509 -key certs/ca.key -out certs/ca.crt -config certs/ca_config.txt
openssl genrsa -out certs/regcred-injector-key.pem 2048
openssl req -new -key certs/regcred-injector-key.pem -subj "/CN=$SERVER_NAME" -out regcred-injector.csr -config certs/ca_config.txt
openssl x509 -req -days 365 -in regcred-injector.csr -CA certs/ca.crt -CAkey certs/ca.key -CAcreateserial -out certs/regcred-injector-crt.pem
