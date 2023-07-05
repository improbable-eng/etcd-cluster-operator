module github.com/improbable-eng/etcd-cluster-operator

go 1.14

// Pin k8s.io/* dependencies to kubernetes-1.17.0 to match controller-runtime v0.5.0
replace (
	// This must match the version below
	go.etcd.io/etcd => go.etcd.io/etcd v0.0.0-20191023171146-3cf2f69b5738
	k8s.io/api => k8s.io/api v0.17.3
	k8s.io/apimachinery => k8s.io/apimachinery v0.17.3
	k8s.io/client-go => k8s.io/client-go v0.17.3
)

require (
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/coreos/go-semver v0.3.0
	github.com/dustin/go-humanize v1.0.0
	github.com/dustinkirkland/golang-petname v0.0.0-20190613200456-11339a705ed2
	github.com/go-logr/logr v0.1.0
	github.com/go-logr/zapr v0.1.1
	github.com/golang/protobuf v1.5.3
	github.com/google/go-cmp v0.5.9
	github.com/google/gofuzz v1.0.0
	github.com/hashicorp/golang-lru v0.5.4 // indirect
	github.com/imdario/mergo v0.3.8 // indirect
	github.com/json-iterator/go v1.1.9 // indirect
	github.com/otiai10/copy v1.0.2
	github.com/prometheus/procfs v0.0.10 // indirect
	github.com/robfig/cron/v3 v3.0.1
	github.com/spf13/pflag v1.0.5
	github.com/stretchr/testify v1.8.3
	// Pin specific etcd version via tag. See https://github.com/etcd-io/etcd/pull/11477
	go.etcd.io/etcd v0.0.0-20191023171146-3cf2f69b5738
	gocloud.dev v0.17.0
	google.golang.org/grpc v1.54.0
	google.golang.org/grpc/examples v0.0.0-20230705174746-11feb0a9afd8 // indirect
	k8s.io/api v0.17.3
	k8s.io/apiextensions-apiserver v0.17.3 // indirect
	k8s.io/apimachinery v0.17.3
	k8s.io/client-go v11.0.0+incompatible
	k8s.io/utils v0.0.0-20191114184206-e782cd3c129f
	sigs.k8s.io/controller-runtime v0.5.0
)
