#!/bin/bash

N=${1:-5}
FASTSYNC=${2:-false}
WEBRTC=${3:-false}
DEST=${4:-"$PWD/conf"}

dest=$DEST/node$N

# Create new key-pair and place it in new conf directory
mkdir -p $dest
echo "Generating key pair for node$N"
go run ../../keygen/keygen.go

# get genesis.peers.json
echo "Fetching peers.genesis.json from node1"
curl -s http://api.decillionai.com:8079/genesispeers > $dest/peers.genesis.json

# get up-to-date peers.json
echo "Fetching peers.json from node1"
curl -s http://api.decillionai.com:8079/peers > $dest/peers.json

cp $dest/peers.json /root/.babble/peers.json
cp $dest/peers.genesis.json /root/.babble/peers.genesis.json

bash run-testnet.sh
