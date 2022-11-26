FROM centos:7
RUN yum install -y wget bind-utils jq fuse && \
     yum clean all && rm -rf /var/cache/yum
ADD https://github.com/krallin/tini/releases/download/v0.19.0/tini-amd64 /bin/tini
RUN chmod +x /bin/tini