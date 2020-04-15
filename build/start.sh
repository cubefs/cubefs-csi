#!/bin/bash

echo "LOG_LEVEL:"${LOG_LEVEL}
echo "CSI_ENDPOINT:"${CSI_ENDPOINT}
echo "KUBE_NODE_NAME:"${KUBE_NODE_NAME}
echo "DRIVER_NAME:"${DRIVER_NAME}
echo "KUBE_CONFIG:"${KUBE_CONFIG}
/cfs/bin/cfs-csi-driver -v=${LOG_LEVEL} --endpoint=${CSI_ENDPOINT} --nodeid=${KUBE_NODE_NAME} --drivername=${DRIVER_NAME} --kubeconfig=${KUBE_CONFIG}> /cfs/logs/cfs-driver.out 2>&1