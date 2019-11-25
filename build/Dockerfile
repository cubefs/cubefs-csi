FROM centos:7
RUN curl -o /etc/yum.repos.d/epel-7.repo http://mirrors.aliyun.com/repo/epel-7.repo && \
        yum install -y wget bind-utils jq fuse
RUN mkdir -p /cfs/bin /cfs/conf /cfs/mnt /cfs/data /cfs/logs
ADD bin/cfs-client /cfs/bin
ADD bin/cfs-csi-driver /cfs/bin
ENTRYPOINT ["/cfs/bin/cfs-csi-driver"]
