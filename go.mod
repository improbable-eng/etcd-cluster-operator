module github.com/improbable-eng/etcd-cluster-operator

go 1.13

// Pin k8s.io/* dependencies to kubernetes-1.15.4 to match controller-runtime v0.3.0
replace (
	k8s.io/api => k8s.io/api v0.0.0-20190918195907-bd6ac527cfd2
	k8s.io/apimachinery => k8s.io/apimachinery v0.0.0-20190817020851-f2f3a405f61d
	k8s.io/apiserver => k8s.io/apiserver v0.0.0-20190918200908-1e17798da8c1
	k8s.io/client-go => k8s.io/client-go v0.0.0-20190918200256-06eb1244587a
)

require (
	github.com/dustinkirkland/golang-petname v0.0.0-20190613200456-11339a705ed2
	github.com/go-logr/logr v0.1.0
	github.com/google/go-cmp v0.3.0
	github.com/stretchr/testify v1.3.0
	go.etcd.io/etcd v3.3.17+incompatible
	k8s.io/api v0.0.0-20190918195907-bd6ac527cfd2
	k8s.io/apimachinery v0.0.0-20190913080033-27d36303b655
	k8s.io/client-go v11.0.0+incompatible
	k8s.io/utils v0.0.0-20190801114015-581e00157fb1
	sigs.k8s.io/controller-runtime v0.3.0
	sigs.k8s.io/kind v0.5.1
)
