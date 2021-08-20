#!/bin/bash

echo "LOG_LEVEL:"${LOG_LEVEL}
echo "CSI_ENDPOINT:"${CSI_ENDPOINT}
echo "KUBE_NODE_NAME:"${KUBE_NODE_NAME}
echo "DRIVER_NAME:"${DRIVER_NAME}
echo "KUBE_CONFIG:"${KUBE_CONFIG}
echo "REMOUNT_DAMAGED:"${REMOUNT_DAMAGED:-false}
echo "KUBELET_ROOT_DIR:"${KUBELET_ROOT_DIR:-/var/lib/kubelet}
/cfs/bin/cfs-csi-driver -v=${LOG_LEVEL} --endpoint=${CSI_ENDPOINT} --nodeid=${KUBE_NODE_NAME} --drivername=${DRIVER_NAME} --kubeconfig=${KUBE_CONFIG} --remountdamaged=${REMOUNT_DAMAGED:-false} --kubeletrootdir=${KUBELET_ROOT_DIR:-/var/lib/kubelet} > /cfs/logs/cfs-driver.out 2>&1