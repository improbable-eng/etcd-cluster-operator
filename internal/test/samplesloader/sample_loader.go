package samplesloader

import (
	"fmt"
	"io/ioutil"
	"path/filepath"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"

	etcdv1alpha1 "github.com/improbable-eng/etcd-cluster-operator/api/v1alpha1"
)

func yamlPathInto(path string, obj runtime.Object) error {
	objBytes, err := ioutil.ReadFile(path)
	if err != nil {
		return fmt.Errorf("error %q reading path %q", err, path)
	}
	scheme := runtime.NewScheme()
	if err := etcdv1alpha1.AddToScheme(scheme); err != nil {
		return fmt.Errorf("error %q adding to scheme", err)
	}
	return runtime.DecodeInto(
		serializer.NewCodecFactory(scheme).UniversalDeserializer(),
		objBytes, obj,
	)
}

// Loader generates deep copies of sample resources for use in tests.
type Loader struct {
	etcdCluster        etcdv1alpha1.EtcdCluster
	etcdPeer           etcdv1alpha1.EtcdPeer
	etcdBackup         etcdv1alpha1.EtcdBackup
	etcdBackupSchedule etcdv1alpha1.EtcdBackupSchedule
	etcdRestore        etcdv1alpha1.EtcdRestore
	namespace          string
}

// New returns a Loader with samples loaded from the `config/samples` manifests.
func New(repoRoot string) Loader {
	basePath := filepath.Join(repoRoot, "config", "samples")
	l := Loader{}

	toLoad := map[string]runtime.Object{
		"cluster":        &l.etcdCluster,
		"peer":           &l.etcdPeer,
		"backup":         &l.etcdBackup,
		"backupschedule": &l.etcdBackupSchedule,
		"restore":        &l.etcdRestore,
	}

	for file, obj := range toLoad {
		if err := yamlPathInto(filepath.Join(basePath, fmt.Sprintf("etcd_v1alpha1_etcd%s.yaml", file)), obj); err != nil {
			panic(err)
		}
	}
	return l
}

// WithNamespace returns a copy Loader with a new default namespace.
func (l Loader) WithNamespace(ns string) Loader {
	l.namespace = ns
	return l
}

// EtcdCluster returns a DeepCopy of a sample EtcdCluster
func (l Loader) EtcdCluster() *etcdv1alpha1.EtcdCluster {
	o := l.etcdCluster.DeepCopy()
	o.Namespace = l.namespace
	return o
}

// EtcdPeer returns a DeepCopy of a sample EtcdPeer
func (l Loader) EtcdPeer() *etcdv1alpha1.EtcdPeer {
	o := l.etcdPeer.DeepCopy()
	o.Namespace = l.namespace
	return o
}

// EtcdBackup returns a DeepCopy of a sample EtcdBackup
func (l Loader) EtcdBackup() *etcdv1alpha1.EtcdBackup {
	o := l.etcdBackup.DeepCopy()
	o.Namespace = l.namespace
	return o
}

// EtcdBackupSchedule returns a DeepCopy of a sample EtcdBackupSchedule
func (l Loader) EtcdBackupSchedule() *etcdv1alpha1.EtcdBackupSchedule {
	o := l.etcdBackupSchedule.DeepCopy()
	o.Namespace = l.namespace
	return o
}

// EtcdRestore returns a DeepCopy of a sample EtcdRestore
func (l Loader) EtcdRestore() *etcdv1alpha1.EtcdRestore {
	o := l.etcdRestore.DeepCopy()
	o.Namespace = l.namespace
	return o
}
