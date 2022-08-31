#!/bin/sh

RootPath=$(
  cd $(dirname $0)
  pwd
)
echo ${RootPath}

CommitID=$(git rev-parse --short=8 HEAD)
Branch=$(git symbolic-ref --short -q HEAD)
BuildTime=$(date +%Y-%m-%dT%H:%M)

CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
  -gcflags=-trimpath=$(pwd) -asmflags=-trimpath=$(pwd) \
  -ldflags="-s -w -X main.CommitID=${CommitID} -X main.BuildTime=${BuildTime} -X main.Branch=${Branch} " \
  -o ${RootPath}/bin/cfs-csi-driver ../cmd && echo "build cfs-csi-driver success"
