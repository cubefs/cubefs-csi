FROM centos

RUN yum install -y fuse
RUN mkdir -p /etc/cfs/

ADD cfs-client /usr/bin/
ADD cfsplugin /

ENTRYPOINT ["/cfsplugin"]