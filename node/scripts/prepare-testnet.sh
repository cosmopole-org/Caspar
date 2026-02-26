#!/bin/bash

docker network create \
  --driver bridge \
  --subnet 10.10.0.0/16 \
  --gateway 10.10.0.1 \
  kasper

bash build-conf.sh 1

mkdir -p /home/kasper/data/docker_proxy/ssl
mkdir /home/kasper/certs
mkdir /home/kasper/packets

openssl req -x509 -newkey ed25519 -days 3650 \
  -noenc -keyout nginx-selfsigned.key -out nginx-selfsigned.crt -subj "/CN=example.com" \
  -addext "subjectAltName=DNS:example.com,DNS:*.example.com,IP:10.0.0.1"

cp nginx-selfsigned.key /home/kasper/data/docker_proxy/ssl/nginx-selfsigned.key
cp nginx-selfsigned.crt /home/kasper/data/docker_proxy/ssl/nginx-selfsigned.crt
