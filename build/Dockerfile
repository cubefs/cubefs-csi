FROM cubefs/cfs-csi-base:v0.0.1
RUN mkdir -p /cfs/bin /cfs/conf /cfs/logs
ADD start.sh /cfs/bin
ADD bin/cfs-client /cfs/bin
ADD bin/cfs-csi-driver /cfs/bin
RUN chmod +x /bin/tini && chmod +x /cfs/bin/cfs-client && chmod +x /cfs/bin/cfs-csi-driver && chmod +x /cfs/bin/start.sh
ENTRYPOINT [ "/bin/tini", "--" ]
CMD [""]