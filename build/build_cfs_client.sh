#!/bin/sh

RootPath=$(cd "$(dirname $0)";pwd)
test -e ${RootPath}/bin/cfs-client && exit 0
test -d ${RootPath}/cubefs && rm -rf ${RootPath}/cubefs
git clone --branch release-3.5.1 https://github.com/cubefs/cubefs.git ${RootPath}/cubefs && cd ${RootPath}/cubefs

# make cubefs client of new version
if [[ -n "$1" ]] ;then
	# checkout the specify branch
    git checkout -b $1 remotes/origin/$1 -f
    exit_code=$?
    if [ ${exit_code} -ne 0 ]; then
        echo "git checkout branch $1 failed:${exit_code}"
        exit ${exit_code}
    fi
fi
new_tag=`git describe --tags --abbrev=0`
git checkout -b $new_tag tags/$new_tag -f

CubeFSSrcPath=${RootPath}/cubefs
mkdir -p ${RootPath}/go-build-cache
echo "building CubeFS client in docker, wait a few minutes......"
Out=`docker run -it --rm --user=$UID:$(id -g $USER) -v ${CubeFSSrcPath}:/go/src/github.com/cubefs/cubefs \
     -v ${RootPath}/go-build-cache:/.cache cubefs/cbfs-base:1.0-golang-1.17.13 \
     /bin/bash -c 'cd /go/src/github.com/cubefs/cubefs/build && bash ./build.sh client > build.out 2>&1 && echo 0 || echo 1'`
if [[ "X${Out:0:1}" != "X0" ]]; then
	echo "build cfs-client fail"
    exit 1
fi

test -d ${RootPath}/bin || mkdir -p ${RootPath}/bin
test -e ${RootPath}/bin/cfs-client && rm ${RootPath}/bin/cfs-client
mv ${CubeFSSrcPath}/build/bin/cfs-client ${RootPath}/bin && rm -rf ${RootPath}/cubefs && rm -rf ${RootPath}/go-build-cache
exit_code=$?
if [ ${exit_code} -ne 0 ]; then
    echo "mv cfs-client failed:${exit_code}"
    exit ${exit_code}
fi
echo "build cfs-client done"


