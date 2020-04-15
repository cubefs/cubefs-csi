#!/bin/sh

RootPath=$(cd $(dirname $0) ; pwd)
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o ${RootPath}/bin/cfs-csi-driver ../cmd && echo "build cfs-csi-driver success"



