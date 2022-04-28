module github.com/improbable-eng/etcd-cluster-operator

go 1.14

// Pin k8s.io/* dependencies to kubernetes-1.20.0 to match controller-runtime v0.8.3
replace (
	// This must match the version below
	go.etcd.io/etcd => go.etcd.io/etcd v0.0.0-20200824191128-ae9734ed278b
	k8s.io/api => k8s.io/api v0.20.2
	k8s.io/apimachinery => k8s.io/apimachinery v0.20.2
	k8s.io/client-go => k8s.io/client-go v0.20.2
)

require (
	github.com/blang/semver v3.5.1+incompatible
	github.com/coreos/go-semver v0.3.0
	github.com/dustin/go-humanize v1.0.0
	github.com/dustinkirkland/golang-petname v0.0.0-20190613200456-11339a705ed2
	github.com/go-logr/logr v0.3.0
	github.com/go-logr/zapr v0.2.0
	github.com/golang/protobuf v1.4.3
	github.com/google/go-cmp v0.5.2
	github.com/google/gofuzz v1.1.0
	github.com/otiai10/copy v1.0.2
	github.com/pkg/errors v0.9.1
	github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring v0.46.0
	github.com/robfig/cron/v3 v3.0.1
	github.com/spf13/pflag v1.0.5
	github.com/stretchr/testify v1.6.1
	// Pin specific etcd version via tag. See https://github.com/etcd-io/etcd/pull/11477
	go.etcd.io/etcd v0.5.0-alpha.5.0.20200910180754-dd1b699fc489
	gocloud.dev v0.17.0
	google.golang.org/grpc v1.27.1
	k8s.io/api v0.20.2
	k8s.io/apimachinery v0.20.2
	k8s.io/client-go v0.20.2
	k8s.io/utils v0.0.0-20210111153108-fddb29f9d009
	sigs.k8s.io/controller-runtime v0.8.3
)
