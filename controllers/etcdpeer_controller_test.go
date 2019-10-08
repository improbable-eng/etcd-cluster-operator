package controllers

import (
	"context"
	"strings"
	"testing"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	etcdv1alpha1 "github.com/improbable-eng/etcd-cluster-operator/api/v1alpha1"
)

func TestStaticBootstrap(t *testing.T) {
	static := etcdv1alpha1.StaticBootstrap{
		InitialCluster: []etcdv1alpha1.InitialClusterMember{
			{
				Name: "foo",
				Host: "foo.bees.default.svc",
			},
			{
				Name: "bar",
				Host: "bar.bees.default.svc",
			},
		},
	}

	actual := staticBootstrapInitialCluster(static)
	expected := "foo=http://foo.bees.default.svc:2380,bar=http://bar.bees.default.svc:2380"

	if expected != actual {
		t.Errorf("Failed to generate correct cluster discovery string. Expected '%s', actual '%s'", expected, actual)
	}
}

var _ = Describe("Etcd peer controller", func() {
	ctx := context.Background()
	SetupTest(ctx)

	Context("ReplicaSet creation", func() {
		It("Should create a ReplicaSet if one does not exist already", func() {

			By("Creating a EtcdPeer")
			etcdPeer := etcdv1alpha1.EtcdPeer{
				ObjectMeta: metav1.ObjectMeta{
					Labels:      make(map[string]string),
					Annotations: make(map[string]string),
					Name:        "bees",
					Namespace:   "default",
				},
				Spec: etcdv1alpha1.EtcdPeerSpec{
					Bootstrap: etcdv1alpha1.Bootstrap{
						Static: etcdv1alpha1.StaticBootstrap{
							InitialCluster: []etcdv1alpha1.InitialClusterMember{
								{
									Name: "foo",
									Host: "foo.default.cluster.local",
								},
								{
									Name: "bar",
									Host: "bar.default.cluster.local",
								},
							},
						},
					},
				},
			}

			err := k8sClient.Create(ctx, &etcdPeer)
			Expect(err).NotTo(HaveOccurred(), "failed to create EtcdPeer resource")

			By("The controller having created a ReplicaSet")
			replicaSet := &appsv1.ReplicaSet{}
			Eventually(
				getResourceFunc(
					ctx,
					client.ObjectKey{
						// Same name and namespace as the EtcdPeer above
						Name:      etcdPeer.Name,
						Namespace: etcdPeer.Namespace,
					},
					replicaSet,
				),
				// Timeout
				time.Second*5,
				// Polling Interval
				time.Millisecond*500,
			).Should(BeNil())

			Expect(*replicaSet.Spec.Replicas).Should(Equal(int32(1)))

			// Find the etcd container
			containers := replicaSet.Spec.Template.Spec.Containers
			var etcdContainer corev1.Container
			for _, container := range containers {
				if container.Name == "etcd" {
					etcdContainer = container
					break
				}
			}
			Expect(etcdContainer).Should(Not(BeNil()))

			image := strings.Split(etcdContainer.Image, ":")[0]
			Expect(image).Should(Equal("quay.io/coreos/etcd"))

			// Find the environment variable for inital cluster
			var etcdInitialClusterEnvVar corev1.EnvVar
			for _, ev := range etcdContainer.Env {
				if ev.Name == "ETCD_INITIAL_CLUSTER" {
					etcdInitialClusterEnvVar = ev
					break
				}
			}
			Expect(etcdContainer).Should(Not(BeNil()))
			Expect(etcdInitialClusterEnvVar.Value).Should(
				Equal("foo=http://foo.default.cluster.local:2380,bar=http://bar.default.cluster.local:2380"))
		})
	})
})

func getResourceFunc(ctx context.Context, key client.ObjectKey, obj runtime.Object) func() error {
	return func() error {
		return k8sClient.Get(ctx, key, obj)
	}
}
