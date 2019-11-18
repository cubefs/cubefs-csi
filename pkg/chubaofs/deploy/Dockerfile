FROM centos
# cfs-client can be built from https://github.com/chubaofs/chubaofs/tree/master/clientv2
RUN yum install wget fuse -y && yum clean all
RUN mkdir -p /etc/cfs/ && \
wget http://storage.jd.com/chubaofs-release/cfs-client && \
chmod +x cfs-client && \
mv cfs-client /usr/bin/cfs-client

ADD cfsplugin /

ENTRYPOINT ["/cfsplugin"]
