#!/usr/bin/env bash

Version="1.0.0"
if [[ -n "$1" ]] ;then
	# docker image tag of CubeFS CSI Driver
    Version=$1
fi

RootPath=$(cd $(dirname $0); pwd)
CfsCsiDriver="cubefs/cfs-csi-driver:$Version"
docker build -t ${CfsCsiDriver} -f ${RootPath}/Dockerfile ${RootPath}
