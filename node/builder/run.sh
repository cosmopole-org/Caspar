#!/bin/bash

rm -r ../src/shell/api/pluggers
rm -r ../src/shell/api/main

go run ./pluggergen.go "../src/shell/api"
