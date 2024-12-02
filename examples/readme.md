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