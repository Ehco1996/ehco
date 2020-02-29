#!/bin/bash
GOOS=linux GOARCH=amd64 go build -o dst/ehco .
GOOS=darwin GOARCH=amd64 go build -o dst/ehco_darwin_amd64 .

