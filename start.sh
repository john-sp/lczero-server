#!/bin/bash

protoc --go_out=. --go_opt=paths=source_relative --go-grpc_out=. --go-grpc_opt=paths=source_relative api/v1/lczero.proto
go build main.go
pkill -f main
nohup ./prod.sh & >server.out
