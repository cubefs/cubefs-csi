# CSI CFS Driver for K8S

## Kubernetes
### Requirements

The following feature gates and runtime config have to be enabled to deploy the driver

```
kubenetes version: 1.15+

kube-apiserver:
--feature-gates=CSIPersistentVolume=true,MountPropagation=true
--runtime-config=api/all

kube-controller-manager:
--feature-gates=CSIPersistentVolume=true

kubelet:
--feature-gates=CSIPersistentVolume=true,MountPropagation=true,KubeletPluginsWatcher=true
--enable-controller-attach-detach=true
```

Mountprogpation requries support for privileged containers. So, make sure privileged containers are enabled in the cluster.

### Get csi sidecar images

```
docker pull quay.io/k8scsi/csi-attacher:v1.0.0
docker pull quay.io/k8scsi/csi-node-driver-registrar:v1.0.2
docker pull quay.io/k8scsi/csi-provisioner:v1.0.0
```

### Build cfscsi driver image

```
docker build -t quay.io/k8scsi/cfscsi:v1.0.0 deploy/.
```

### Create configmap for csi driver

```
kubectl create configmap kubecfg --from-file=deploy/kubernetes/kubecfg
```

### Create RBAC rules (ServiceAccount, ClusterRole, ClusterRoleBinding) and StorageClass
```
kubectl apply -f deploy/dynamic_provision/cfs-rbac.yaml
kubectl apply -f deploy/dynamic_provision/cfs-sc.yaml
```

### Deploy cfs csi-driver by sidecar in the same pod
```
kubectl apply -f deploy/dynamic_provision/sidecar/cfs-sidecar.yaml
```

### Deploy cfs csi-controller and csi-node independently 
```
kubectl apply -f deploy/dynamic_provision/independent/csi-controller-statefulset.yaml
kubectl apply -f deploy/dynamic_provision/independent/csi-node-daemonset.yaml
```

### Create pvc
```
kubectl apply -f deploy/dynamic_provision/cfs-pvc.yaml
```


### Pre Volume: you must know volumeName first, example Nginx application

Please update the cfs Master environment information: 'MASTER_ADDRESS' in all files.

### Dynamic volume: Example Nginx application

```
docker pull nginx
kubectl apply -f deploy/dynamic_provision/pv-pod.yaml
```
