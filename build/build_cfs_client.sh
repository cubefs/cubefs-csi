#!/bin/sh

RootPath=$(cd "$(dirname $0)";pwd)
test -e ${RootPath}/bin/cfs-client && exit 0
test -d ${RootPath}/chubaofs && rm -rf ${RootPath}/chubaofs
git clone https://github.com/chubaofs/chubaofs.git ${RootPath}/chubaofs && cd ${RootPath}/chubaofs 
ChubaoFSSrcPath=${RootPath}/chubaofs
Out=`docker run -it --rm --privileged -v ${ChubaoFSSrcPath}:/root/go/src/github.com/chubaofs/chubaofs chubaofs/cfs-base:1.0.1 \
     /bin/bash -c 'cd /root/go/src/github.com/chubaofs/chubaofs/build && bash ./build.sh > build.out 2>&1 && echo 0 || echo 1'`
if [[ "X${Out:0:1}" != "X0" ]]; then
	echo "build cfs-client fail"
    exit 1
fi

test -d ${RootPath}/bin || mkdir -p ${RootPath}/bin
test -e ${RootPath}/bin/cfs-client && rm ${RootPath}/bin/cfs-client
mv ${ChubaoFSSrcPath}/build/bin/cfs-client ${RootPath}/bin && rm -rf ${RootPath}/chubaofs
echo "build cfs-client success"


