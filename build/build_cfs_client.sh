#!/bin/sh

RootPath=$(cd "$(dirname $0)";pwd)
test -e ${RootPath}/bin/cfs-client && exit 0
test -d ${RootPath}/cubefs && rm -rf ${RootPath}/cubefs
git clone https://github.com/cubefs/cubefs.git ${RootPath}/cubefs && cd ${RootPath}/cubefs 
CubeFSSrcPath=${RootPath}/cubefs
Out=`docker run -it --rm --privileged -v ${CubeFSSrcPath}:/root/go/src/github.com/cubefs/cubefs cubefs/cfs-base:1.0.1 \
     /bin/bash -c 'cd /root/go/src/github.com/cubefs/cubefs/build && bash ./build.sh > build.out 2>&1 && echo 0 || echo 1'`
if [[ "X${Out:0:1}" != "X0" ]]; then
	echo "build cfs-client fail"
    exit 1
fi

test -d ${RootPath}/bin || mkdir -p ${RootPath}/bin
test -e ${RootPath}/bin/cfs-client && rm ${RootPath}/bin/cfs-client
mv ${CubeFSSrcPath}/build/bin/cfs-client ${RootPath}/bin && rm -rf ${RootPath}/cubefs
echo "build cfs-client success"


