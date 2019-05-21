# CSI CFS Driver for K8S

## Kubernetes
### Requirements

The folllowing feature gates and runtime config have to be enabled to deploy the driver

```
kubenetes version: 1.12.5

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
docker pull quay.io/k8scsi/csi-attacher:v0.3.0
docker pull quay.io/k8scsi/driver-registrar:v0.3.0
docker pull quay.io/k8scsi/csi-provisioner:v0.3.0
```

### Build cfscsi driver image

```docker build -t cfscsi:v2 deploy/.```

### Create configmap for csi driver

```kubectl create configmap kubecfg --from-file=deploy/kubernetes/kubecfg```

### Deploy csi-sidecar and cfs csi-driver

```kubectl apply -f deploy/dynamic_provision/cfs-rbac.yaml```
```kubectl apply -f deploy/dynamic_provision/cfs-sc.yaml```
```kubectl apply -f deploy/dynamic_provision/cfs-pvc.yaml```
```kubectl apply -f deploy/dynamic_provision/cfs-sidecar.yaml```

### Pre Volume: you must know volumeName first, example Nginx application

Please update the cfs Master Hosts & volumeName information in pv-pod.yaml file.

### Dynamic volume: Example Nginx application

```
docker pull nginx
kubectl apply -f deploy/dynamic_provision/pv-pod.yaml
```
