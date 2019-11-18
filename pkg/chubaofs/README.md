# CSI CFS Driver for K8S

## Kubernetes
### 1. Requirements

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

### 2. Get csi sidecar images

```
docker pull quay.io/k8scsi/csi-attacher:v1.0.0
docker pull quay.io/k8scsi/csi-node-driver-registrar:v1.0.2
docker pull quay.io/k8scsi/csi-provisioner:v1.0.0
```

### 3. get cfscsi driver image in two ways

* pull cfscsi driver image from docker.io

```docker pull docker.io/chubaofs/cfscsi:v1.0.0```

* Build cfscsi driver image yourself

```
make -C ../../ cfs-image  
```

### 4. Create kubeconfig for csi driver

```kubectl create configmap kubecfg --from-file=deploy/kubernetes/kubecfg```

### 5. Create RBAC rules (ServiceAccount, ClusterRole, ClusterRoleBinding) and StorageClass
```
kubectl apply -f deploy/dynamic_provision/cfs-rbac.yaml
kubectl apply -f deploy/dynamic_provision/cfs-sc.yaml
```
### 6. Pre Volume:

* Update the real cfs MasterAddress: 'MASTER_ADDRESS' in yaml files.
* Update <ControllerServer IP> and <NodeServer IP> in yaml files.

### 7. Deploy cfs csi-driver in two ways

* Deploy cfs csi-driver by sidecar
```
kubectl apply -f deploy/dynamic_provision/sidecar/cfs-sidecar.yaml
```

* Deploy cfs csi-driver by csi-controller-statefulset and csi-node-daemonset
```
kubectl label nodes <ControllerServer IP> csi-role=node
kubectl label nodes <NodeServer IP> csi-role=controller
kubectl apply -f deploy/dynamic_provision/independent/csi-controller-statefulset.yaml
kubectl apply -f deploy/dynamic_provision/independent/csi-node-daemonset.yaml
```

### 8. Create pvc
```
kubectl apply -f deploy/dynamic_provision/cfs-pvc.yaml
```

### 9. Dynamic volume: Example Nginx application

```
docker pull nginx
kubectl apply -f deploy/dynamic_provision/pv-pod.yaml
```
