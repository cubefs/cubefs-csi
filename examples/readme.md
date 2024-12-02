kubectl  edit clusterrole system:node-proxier 
- apiGroups:
    - storage.k8s.io
      resources:
    - csinodes
    - csidrivers
    - volumeattachments  -->新增
      verbs:
    - '*'
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
name: system:nodes:volumeattachments
rules:
- apiGroups:
    - storage.k8s.io
      resources:
    - volumeattachments
      verbs:
    - create
    - watch

---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
name: system:nodes:volumeattachments
roleRef:
apiGroup: rbac.authorization.k8s.io
kind: ClusterRole
name: system:nodes:volumeattachments
subjects:
- apiGroup: rbac.authorization.k8s.io
  kind: Group
  name: system:nodes

docker run -it -v $(pwd):/go/src/github.com/cubefs/cubefs-csi --platform linux/amd64 golang:1.18.10
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -gcflags=-trimpath=$(pwd) -asmflags=-trimpath=$(pwd) -o build/bin/cfs-csi-driver ./cmd/
docker build --platform linux/amd64 -t cfs-csi-driver:v3.3.2-2 ./build