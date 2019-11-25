#!/bin/sh

RootPath=$(cd "$(dirname $0)";pwd)
CfsClientVersion=1.4.0
test -e ${RootPath}/bin/cfs-client && exit 0
test -d ${RootPath}/chubaofs && rm -rf ${RootPath}/chubaofs
git clone https://github.com/chubaofs/chubaofs.git && cd ${RootPath}/chubaofs && git checkout v${CfsClientVersion}
ChubaoFSSrcPath=${RootPath}/chubaofs
Out=`docker run -it --rm --privileged -v ${ChubaoFSSrcPath}:/go/src/github.com/chubaofs/chubaofs chubaofs/cfs-base:1.0 \
    bash -c 'cd /go/src/github.com/chubaofs/chubaofs/client && bash ./build.sh && echo 0 || echo 1'`
if [[ "X${Out:0:1}" != "X0" ]]; then
	echo "build cfs-client fail"
	exit 1
fi

test -d ${RootPath}/bin || mkdir -p ${RootPath}/bin
test -e ${RootPath}/bin/cfs-client && rm ${RootPath}/bin/cfs-client
mv ${ChubaoFSSrcPath}/client/cfs-client ${RootPath}/bin && rm -rf ${RootPath}/chubaofs
echo "build cfs-client success"


