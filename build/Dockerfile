FROM centos:7.2.1511
RUN curl -o /etc/yum.repos.d/epel-7.repo http://mirrors.aliyun.com/repo/epel-7.repo && \
        yum install -y wget bind-utils jq fuse
RUN mkdir -p /cfs/bin /cfs/conf /cfs/logs
ADD bin/cfs-client /cfs/bin
ADD bin/cfs-csi-driver /cfs/bin
ADD start.sh /cfs/bin
