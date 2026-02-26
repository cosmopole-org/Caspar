#!/bin/bash

now=$(date +"%m_%d_%Y")
sudo docker exec midodb pg_dump -h localhost -U postgres postgres > "/home/ubuntu/database/backup-${now}.sql"

gdrive files upload "/home/ubuntu/database/backup-${now}.sql"
rm "/home/ubuntu/database/backup-${now}.sql"
