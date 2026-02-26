#!/bin/bash
set -e

# Variables
QUESTDB_DIR="/app/questdb"

# Start QuestDB in background
echo "Starting QuestDB..."
$QUESTDB_DIR/questdb.sh start -d $QUESTDB_DIR -f
