FROM centos:7.2.1511
RUN curl -o /etc/yum.repos.d/epel-7.repo http://mirrors.aliyun.com/repo/epel-7.repo && \
        yum install -y wget bind-utils jq fuse
RUN mkdir -p /cfs/bin /cfs/conf /cfs/logs
ADD https://github.com/krallin/tini/releases/download/v0.19.0/tini-amd64 /bin/tini
ADD bin/cfs-client /cfs/bin
ADD bin/cfs-csi-driver /cfs/bin
ADD start.sh /cfs/bin
RUN chmod +x /bin/tini && chmod +x /cfs/bin/cfs-client && chmod +x /cfs/bin/cfs-csi-driver && chmod +x /cfs/bin/start.sh
ENTRYPOINT [ "/bin/tini", "--" ]
CMD [""]