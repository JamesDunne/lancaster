#!/bin/bash
GOOS=darwin go build -o lancaster
GOOS=windows GOARCH=amd64 go build -o lancaster.exe
