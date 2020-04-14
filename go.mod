module github.com/chubaofs/chubaofs-csi

go 1.13

require (
	github.com/container-storage-interface/spec v0.3.0
	github.com/docker/distribution v2.7.1+incompatible // indirect
	github.com/ghodss/yaml v1.0.0 // indirect
	github.com/gogo/protobuf v1.3.1 // indirect
	github.com/golang/glog v0.0.0-20160126235308-23def4e6c14b
	github.com/golang/groupcache v0.0.0-20200121045136-8c9f03a8e57e // indirect
	github.com/golang/protobuf v1.3.5 // indirect
	github.com/google/btree v1.0.0 // indirect
	github.com/google/gofuzz v1.1.0 // indirect
	github.com/google/uuid v1.1.1 // indirect
	github.com/googleapis/gnostic v0.4.0 // indirect
	github.com/gregjones/httpcache v0.0.0-20190611155906-901d90724c79 // indirect
	github.com/hashicorp/golang-lru v0.5.4 // indirect
	github.com/inconshreveable/mousetrap v1.0.0 // indirect
	github.com/json-iterator/go v1.1.9 // indirect
	github.com/kubernetes-csi/csi-lib-utils v0.2.0
	// github.com/kubernetes-csi/drivers v0.4.2 // indirect
	github.com/onsi/ginkgo v1.12.0 // indirect
	github.com/onsi/gomega v1.9.0 // indirect
	github.com/opencontainers/go-digest v1.0.0-rc1 // indirect
	github.com/pborman/uuid v0.0.0-20180906182336-adf5a7427709 // indirect
	github.com/peterbourgon/diskv v2.0.1+incompatible // indirect
	github.com/prometheus/client_golang v1.1.0 // indirect
	github.com/prometheus/common v0.9.1 // indirect
	github.com/prometheus/procfs v0.0.11 // indirect
	github.com/spf13/cobra v0.0.1
	github.com/spf13/pflag v1.0.5 // indirect
	github.com/stretchr/testify v1.5.1
	golang.org/x/crypto v0.0.0-20200406173513-056763e48d71 // indirect
	golang.org/x/net v0.0.0-20200324143707-d3edc9973b7e
	golang.org/x/oauth2 v0.0.0-20200107190931-bf48bf16ab8d // indirect
	golang.org/x/sys v0.0.0-20200408040146-ea54a3c99b9b // indirect
	golang.org/x/time v0.0.0-20191024005414-555d28b269f0 // indirect
	google.golang.org/appengine v1.6.5 // indirect
	google.golang.org/genproto v0.0.0-20200407120235-9eb9bb161a06 // indirect
	google.golang.org/grpc v1.28.1
	gopkg.in/inf.v0 v0.9.1 // indirect
	gopkg.in/square/go-jose.v2 v2.4.1 // indirect
	k8s.io/api v0.0.0-20181004124137-fd83cbc87e76
	k8s.io/apiextensions-apiserver v0.0.0-20181004124836-1748dfb29e8a // indirect
	k8s.io/apimachinery v0.0.0-20180913025736-6dd46049f395
	k8s.io/apiserver v0.0.0-20181004124341-e85ad7b666fe // indirect
	k8s.io/client-go v9.0.0+incompatible
	k8s.io/cloud-provider v0.0.0-20181221204816-2325825fd8d8 // indirect
	k8s.io/csi-api v0.0.0-20181004125007-daa9d551756f // indirect
	k8s.io/klog v0.3.0
	k8s.io/kube-openapi v0.0.0-20190115222348-ced9eb3070a5 // indirect
	k8s.io/kubernetes v1.12.0 // indirect
	k8s.io/utils v0.0.0-20200327001022-6496210b90e8
	sigs.k8s.io/yaml v1.2.0 // indirect
)

replace (
	golang.org/x/crypto v0.0.0-20200406173513-056763e48d71 => github.com/golang/crypto v0.0.0-20190313024323-a1f597ede03a // indirect
	golang.org/x/net v0.0.0-20200324143707-d3edc9973b7e => github.com/golang/net v0.0.0-20200324143707-d3edc9973b7e
	golang.org/x/oauth2 v0.0.0-20200107190931-bf48bf16ab8d => github.com/golang/oauth2 v0.0.0-20200107190931-bf48bf16ab8d // indirect
	golang.org/x/sys v0.0.0-20200408040146-ea54a3c99b9b => github.com/golang/sys v0.0.0-20200408040146-ea54a3c99b9b // indirect
	golang.org/x/time v0.0.0-20191024005414-555d28b269f0 => github.com/golang/time v0.0.0-20191024005414-555d28b269f0 // indirect
	google.golang.org/appengine v1.6.5 => github.com/golang/appengine v1.6.5 // indirect
	google.golang.org/genproto v0.0.0-20200407120235-9eb9bb161a06 => github.com/googleapis/go-genproto v0.0.0-20200407120235-9eb9bb161a06 // indirect
	google.golang.org/grpc v1.28.1 => github.com/grpc/grpc-go v1.28.1
	// gopkg.in/inf.v0 v0.9.1 // indirect
	// gopkg.in/square/go-jose.v2 v2.4.1 // indirect
	k8s.io/api v0.0.0-20181004124137-fd83cbc87e76 => github.com/kubernetes/api v0.0.0-20181004124137-fd83cbc87e76
	k8s.io/apiextensions-apiserver v0.0.0-20181004124836-1748dfb29e8a => github.com/kubernetes/apiextensions-apiserver v0.0.0-20181004124836-1748dfb29e8a // indirect
	k8s.io/apimachinery v0.0.0-20180913025736-6dd46049f395 => github.com/kubernetes/apimachinery v0.0.0-20180913025736-6dd46049f395
	k8s.io/apiserver v0.0.0-20181004124341-e85ad7b666fe => github.com/kubernetes/apiserver v0.0.0-20181004124341-e85ad7b666fe // indirect
	k8s.io/client-go v9.0.0+incompatible => github.com/kubernetes/client-go v9.0.0+incompatible
	k8s.io/cloud-provider v0.0.0-20181221204816-2325825fd8d8 => github.com/kubernetes/cloud-provider v0.0.0-20181221204816-2325825fd8d8 // indirect
	k8s.io/csi-api v0.0.0-20181004125007-daa9d551756f => github.com/kubernetes/csi-api v0.0.0-20181004125007-daa9d551756f // indirect
	k8s.io/klog v0.3.0 => github.com/kubernetes/klog v0.3.0
	k8s.io/kube-openapi v0.0.0-20190115222348-ced9eb3070a5 => github.com/kubernetes/kube-openapi v0.0.0-20190115222348-ced9eb3070a5 // indirect
	k8s.io/kubernetes v1.12.0 => github.com/kubernetes/kubernetes v1.12.0
	k8s.io/utils v0.0.0-20200327001022-6496210b90e8 => github.com/kubernetes/utils v0.0.0-20200327001022-6496210b90e8
	sigs.k8s.io/yaml v1.2.0 => github.com/kubernetes-sigs/yaml v1.2.0 // indirect
)
