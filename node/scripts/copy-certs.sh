#!/bin/bash

SRC="/etc/letsencrypt/live/yourdomain.com"
DEST="/path/to/your/target-folder"

mkdir -p "$DEST"
cp -L "$SRC/fullchain.pem" "$DEST/fullchain.pem"
cp -L "$SRC/privkey.pem" "$DEST/privkey.pem"
chown root:root "$DEST"/*.pem
chmod 600 "$DEST"/*.pem

