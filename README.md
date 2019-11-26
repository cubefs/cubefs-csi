# ChubaoFS CSI

## Prerequisite

* Kubernetes 1.16.0
* ChubaoFS 1.4.0

## Enable privileged Pods

To use CSI drivers, your Kubernetes cluster must allow privileged pods (i.e. --allow-privileged flag must be set to true for both the API server and the kubelet). Ensure your API server are started with the privileged flag.

```
$ ./kube-apiserver ...  --allow-privileged=true ...
$ ./kubelet ...  --allow-privileged=true ...
```

Note: Starting from Kubernetes 1.13.0, --allow-privileged is true for kubelet. It'll be deprecated in future kubernetes releases. Please refer to [Kubernetes CSI Deploy](https://kubernetes-csi.github.io/docs/deploying.html) for more details.

## Prepare on-premise ChubaoFS cluster

An on-premise ChubaoFS cluster can be deployed seperately, or within the same Kubernetes cluster as applications which requrie persistent volumes. Please refer to [chubaofs-helm](https://github.com/chubaofs/chubaofs-helm) for more details on deployment using Helm.

## Deploy the CSI driver

```
$ kubectl apply -f deploy/csi-controller-deployment.yaml
$ kubectl apply -f deploy/csi-node-daemonset.yaml
```
## Connect Pod to ChubaoFS cluster

There are several abstraction layers between Pods and on-premise ChubaoFS cluster, and we are going to establish the connection through following steps for a Pod to use ChubaoFS as its backend storage.

1. Create StorageClass
2. Create PVC (Persistent Volume Claim)
3. Refer to a PVC in Pod

### Create StorageClass

An example storage class yaml file looks like below.

```yaml
kind: StorageClass
apiVersion: storage.k8s.io/v1
metadata:
  name: chubaofs-sc
provisioner: csi.chubaofs.com
reclaimPolicy: Delete
parameters:
  masterAddr: "master-service.chubaofs.svc.cluster.local:8080"
  owner: "csi-user"
  consulAddr: "consul-service.chubaofs.svc.cluster.local:8500"
  logLevel: "debug"
```

Use the following command to create.

```
$ kubectl create -f ~/storageclass-chubaofs.yaml
```

The field `provisioner` indicates name of the CSI driver, which is `csi.chubaofs.com` in this example. It is connected to the `drivername` in `deploy/csi-controller-deployment.yaml` and `deploy/csi-node-daemonset.yaml`. So StorageClass knows which driver should be used to manipulate the backend storage cluster.

| Name       | Madotory | Description|
| :--------- | :------: | ---------: |
| MasterAddr | Y | Master address of a specific on-premise ChubaoFS cluster |
| consulAddr | N | |


### Create PVC

An example pvc yaml file looks like below.

```yaml
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: chubaofs-pvc
spec:
  accessModes:
    - ReadWriteOnce
  resources:
    requests:
      storage: 5Gi
  storageClassName: chubaofs-sc
```

```
$ kubectl create -f ~/pvc.yaml
```

The field `storageClassName` refers to the StorageClass we already created.

### Use PVC in a Pod

The example `nginx-deployment.yaml` looks like below.

```yaml
...
    spec:
      containers:
        - name: csi-demo
          image: alpine:3.10.3
          volumeMounts:
            - name: mypvc
              mountPath: /data
      volumes:
        - name: mypvc
          persistentVolumeClaim:
            claimName: chubaofs-pvc
...
```

The field `claimName` refers to the PVC we already created.
```
$ kubectl create -f ~/nginx-deployment.yaml
```