#!/bin/bash

echo "LOG_LEVEL:"${LOG_LEVEL}
echo "CSI_ENDPOINT:"${CSI_ENDPOINT}
echo "KUBE_NODE_NAME:"${KUBE_NODE_NAME}
echo "DRIVER_NAME:"${DRIVER_NAME}
nohup /cfs/bin/cfs-csi-driver -v=${LOG_LEVEL} --endpoint=${CSI_ENDPOINT} --nodeid=${KUBE_NODE_NAME} --drivername=${DRIVER_NAME} > /cfs/logs/cfs-driver.out 2>&1 &
sleep 9999999d
