#!/usr/bin/env bash

Version="v1.0.0"
if [[ -n "$1" ]] ;then
	# docker image tag of CubeFS CSI Driver, e.g. v3.3.0
    Version=$1
fi

RootPath=$(cd $(dirname $0); pwd)
CfsCsiDriver="cubefs/cfs-csi-driver:$Version"
docker build -t ${CfsCsiDriver} -f ${RootPath}/Dockerfile ${RootPath}
