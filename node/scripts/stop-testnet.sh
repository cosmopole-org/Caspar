#!/bin/bash

docker ps -f kasper-proxy -f name=logsdb -f name=node -aq | xargs docker stop | xargs docker rm -f 

docker network rm kasper

rm /home/keyhan/data/docker_proxy/nginx.conf