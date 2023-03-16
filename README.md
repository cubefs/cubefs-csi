[![Build Status](https://travis-ci.org/cubefs/cubefs-csi.svg?branch=master)](https://travis-ci.org/cubefs/cubefs-csi)

# CubeFS CSI Driver

## Overview

CubeFS Container Storage Interface (CSI) plugins.

## Prerequisite

* Kubernetes 1.16.0
* CSI spec version 1.1.0

## Prepare on-premise CubeFS cluster

An on-premise CubeFS cluster can be deployed separately, or within the same Kubernetes cluster as applications which require persistent volumes. Please refer to [cubefs-helm](https://github.com/cubefs/cubefs-helm) for more details on deployment using Helm.



# Deploy

CSI supports deploy with helm as well as using raw YAML files.

Though, the first step of these two methods are label the node:

## Add labels to Kubernetes node

You should tag each Kubernetes node with the appropriate labels accorindly for CSI node of CubeFS.
`deploy/csi-controller-deployment.yaml` and `deploy/csi-node-daemonset.yaml` have `nodeSelector` element,
so you should add a label for nodes. If you want using CubeFS CSI in whole kubernetes cluster, you can delete `nodeSelector` element.

```
kubectl label node <nodename> component.cubefs.io/csi=enabled
```

##

## Direct Raw Files Deployment

### Deploy the CSI driver

```
$ kubectl apply -f deploy/csi-rbac.yaml
$ kubectl apply -f deploy/csi-controller-deployment.yaml
$ kubectl apply -f deploy/csi-node-daemonset.yaml
```
> **Notes:** If your kubernetes cluster alter the kubelet path `/var/lib/kubelet` to other path(such as: `/data1/k8s/lib/kubelet`), you must execute the following commands to update the path:
>
> `sed -i 's#/var/lib/kubelet#/data1/k8s/lib/kubelet#g'  deploy/csi-controller-deployment.yaml`
>
> `sed -i 's#/var/lib/kubelet#/data1/k8s/lib/kubelet#g'  deploy/csi-node-daemonset.yaml`

### Use Remote CubeFS Cluster as backend storage

There is only 3 steps before finally using remote CubeFS cluster as file system

1. Create StorageClass
2. Create PVC (Persistent Volume Claim)
3. Reference PVC in a Pod

### Create StorageClass

An example storage class yaml file is shown below.

```yaml
kind: StorageClass
apiVersion: storage.k8s.io/v1
metadata:
  name: cfs-sc
provisioner: csi.cubefs.com
reclaimPolicy: Delete
parameters:
  masterAddr: "master-service.cubefs.svc.cluster.local:17010"
  consulAddr: "http://consul-service.cubefs.svc.cluster.local:8500"
  owner: "csiuser"
  logLevel: "debug"
```

Creating command.

```
$ kubectl create -f deploy/storageclass.yaml
```



## Helm Deployment

### Download the CubeFS-Helm project

```
git clone https://github.com/cubefs/cubefs-helm
cd cubefs-helm
```

### Edit the values file

Create a values file, and edit it as below:

`vi ~/cubefs.yaml`

```
component:
  master: false
  datanode: false
  metanode: false
  objectnode: false
  client: false
  csi: true
  monitor: false
  ingress: false

image:
  # CSI related images
  csi_driver: ghcr.io/cubefs/cfs-csi-driver:3.2.0.150.0
  csi_provisioner: registry.k8s.io/sig-storage/csi-provisioner:v2.2.2
  csi_attacher: registry.k8s.io/sig-storage/csi-attacher:v3.4.0
  csi_resizer: registry.k8s.io/sig-storage/csi-resizer:v1.3.0
  driver_registrar: registry.k8s.io/sig-storage/csi-node-driver-registrar:v2.5.0

csi:
  driverName: csi.cubefs.com
  logLevel: error
  # If you changed the default kubelet home path, this
  # value needs to be modified accordingly
  kubeletPath: /var/lib/kubelet
  controller:
    tolerations: [ ]
    nodeSelector:
      "component.cubefs.io/csi": "enabled"
  node:
    tolerations: [ ]
    nodeSelector:
      "component.cubefs.io/csi": "enabled"
    resources:
      enabled: false
      requests:
        memory: "4048Mi"
        cpu: "2000m"
      limits:
        memory: "4048Mi"
        cpu: "2000m"
  storageClass:
    # Whether automatically set this StorageClass to default volume provisioner
    setToDefault: true
    # StorageClass reclaim policy, 'Delete' or 'Retain' is supported
    reclaimPolicy: "Delete"
    # Override the master address parameter to connect to external cluster, if the cluster is deployed
    # in the same k8s cluster, it can be omitted.
    masterAddr: ""
    otherParameters:
```

### Install

`helm upgrade --install cubefs ./cubefs -f ~/cubefs.yaml -n cubefs --create-namespace`

## Verify

After we installed the CSI, we can create a PVC and mount it inside a Pod to verify if everything all right.

### Create PVC

An example pvc yaml file is shown below.

```yaml
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: cfs-pvc
spec:
  accessModes:
  - ReadWriteMany
  volumeMode: Filesystem
  resources:
    requests:
      storage: 5Gi
  storageClassName: cfs-sc
```

```
$ kubectl create -f example/pvc.yaml
```

The field `storageClassName` refers to the StorageClass we already created.

### Use PVC in a Pod

The example `deployment.yaml` looks like below.

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: cfs-csi-demo
  namespace: default
spec:
  replicas: 1
  selector:
    matchLabels:
      app: cfs-csi-demo-pod
  template:
    metadata:
      labels:
        app: cfs-csi-demo-pod
    spec:
      containers:
        - name: cubefs-csi-demo
          image: nginx:1.17.9
          imagePullPolicy: "IfNotPresent"
          ports:
            - containerPort: 80
              name: "http-server"
          volumeMounts:
            - mountPath: "/usr/share/nginx/html"
              name: mypvc
      volumes:
        - name: mypvc
          persistentVolumeClaim:
            claimName: cfs-pvc
```

The field `claimName` refers to the PVC created before.
```
$ kubectl create -f examples/deployment.yaml
```
