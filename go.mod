module github.com/improbable-eng/etcd-cluster-operator

go 1.13

// Pin k8s.io/* dependencies to kubernetes-1.16.0 to match controller-runtime v0.4.0
replace (
	k8s.io/api => k8s.io/api v0.0.0-20190918155943-95b840bb6a1f
	k8s.io/apiextensions-apiserver => k8s.io/apiextensions-apiserver v0.0.0-20190918161926-8f644eb6e783
	k8s.io/apimachinery => k8s.io/apimachinery v0.0.0-20190913080033-27d36303b655
	k8s.io/client-go => k8s.io/client-go v0.0.0-20190918160344-1fbdaa4c8d90
	k8s.io/utils => k8s.io/utils v0.0.0-20190801114015-581e00157fb1
)

require (
	cloud.google.com/go v0.39.0
	github.com/coreos/etcd v3.3.18+incompatible
	github.com/dustinkirkland/golang-petname v0.0.0-20190613200456-11339a705ed2
	github.com/go-logr/logr v0.1.0
	github.com/google/go-cmp v0.3.0
	github.com/google/gofuzz v1.0.0
	github.com/otiai10/copy v1.0.2
	github.com/robfig/cron/v3 v3.0.0
	github.com/spf13/viper v1.3.2
	github.com/stretchr/testify v1.4.0
	// Not upgradable. See https://github.com/etcd-io/etcd/issues/11154
	go.etcd.io/etcd v3.3.17+incompatible
	go.opencensus.io v0.22.1 // indirect
	go.uber.org/zap v1.10.0
	gocloud.dev v0.18.0
	google.golang.org/api v0.13.0 // indirect
	k8s.io/api v0.0.0-20190918195907-bd6ac527cfd2
	k8s.io/apimachinery v0.0.0-20190913080033-27d36303b655
	k8s.io/client-go v11.0.0+incompatible
	k8s.io/utils v0.0.0-20190801114015-581e00157fb1
	sigs.k8s.io/controller-runtime v0.4.0
	sigs.k8s.io/kind v0.5.1
)
