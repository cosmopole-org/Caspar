#!/bin/bash

set -e

N=${1:-4}
WEBRTC=${2:-false}
DEST=${3:-"$PWD/conf"}
IPBASE=${4:-172.77.5.}
PORT=${5:-1337}


dest=$DEST/node1
mkdir -p $dest

echo "api.decillionai.com:$PORT" > $dest/addr

echo "Generating key pair for node$i"
go run ../keygen/keygen.go

PFILE=$DEST/peers.json
echo "[" > $PFILE
com=","
if [[ 1 == $N ]]; then
    com=""
fi

printf "\t{\n" >> $PFILE
if "$WEBRTC"; then
    printf "\t\t\"NetAddr\":\"$(cat $DEST/node1/key.pub)\",\n" >> $PFILE
else
    printf "\t\t\"NetAddr\":\"$(cat $DEST/node1/addr)\",\n" >> $PFILE
fi
printf "\t\t\"PubKeyHex\":\"$(cat /root/.babble/key.pub)\",\n" >> $PFILE
printf "\t\t\"Moniker\":\"node1\"\n" >> $PFILE
printf "\t}%s\n"  $com >> $PFILE

echo "]" >> $PFILE

dest=$DEST/node1
cp $DEST/peers.json $dest/
cp $DEST/peers.json $dest/peers.genesis.json

cp $DEST/peers.json /root/.babble/peers.json
cp $DEST/peers.json /root/.babble/peers.genesis.json