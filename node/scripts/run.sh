#!/bin/bash

bash /app/scripts/run-fcvmm.sh
bash /app/scripts/run-questdb.sh &
/app/appengine/target/debug/appengine &

/app/kasper
