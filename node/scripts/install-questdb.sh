#!/bin/bash
set -e

# Variables
QUESTDB_VERSION="9.1.0"
QUESTDB_DIR="/app/questdb"
QUESTDB_PORT=9000
PG_PORT=5432

# Download QuestDB if not exists
if [ ! -d "$QUESTDB_DIR" ]; then
    echo "Downloading QuestDB $QUESTDB_VERSION..."
    curl -L https://github.com/questdb/questdb/releases/download/9.1.0/questdb-$QUESTDB_VERSION-no-jre-bin.tar.gz -o /tmp/questdb.tar.gz
    tar -xzf /tmp/questdb.tar.gz -C /app
    mv /app/questdb-$QUESTDB_VERSION-no-jre-bin $QUESTDB_DIR
fi
