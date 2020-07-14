module github.com/chubaofs/chubaofs-csi

go 1.13

require (
	github.com/container-storage-interface/spec v1.1.0
	github.com/golang/glog v0.0.0-20160126235308-23def4e6c14b
	github.com/kubernetes-csi/csi-lib-utils v0.7.0
	github.com/spf13/cobra v1.0.0
	github.com/stretchr/testify v1.6.1
	golang.org/x/net v0.0.0-20200602114024-627f9648deb9
	google.golang.org/grpc v1.29.1
	k8s.io/api v0.17.0
	k8s.io/apimachinery v0.17.1-beta.0
	k8s.io/client-go v0.17.0
	k8s.io/utils v0.0.0-20191114184206-e782cd3c129f
)

replace (
	k8s.io/api => github.com/kubernetes/api v0.16.4
	k8s.io/apimachinery => github.com/kubernetes/apimachinery v0.16.4 // indirect
	k8s.io/client-go => github.com/kubernetes/client-go v0.16.4
)
