package controllers

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	etcdv1alpha1 "github.com/improbable-eng/etcd-cluster-operator/api/v1alpha1"
)

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
		})
	})
})

func getResourceFunc(ctx context.Context, key client.ObjectKey, obj runtime.Object) func() error {
	return func() error {
		return k8sClient.Get(ctx, key, obj)
	}
}
