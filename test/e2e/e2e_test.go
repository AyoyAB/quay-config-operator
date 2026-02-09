//go:build e2e

package e2e

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	quayv1alpha1 "github.com/AyoyAB/quay-config-operator/api/v1alpha1"
)

const testNamespace = "default"

func createCredentialsSecret(ctx context.Context, name string) {
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: testNamespace,
		},
		StringData: map[string]string{
			"host":          "http://quay-mock.quay-system.svc.cluster.local",
			"token":         "e2e-test-token",
			"validateCerts": "false",
		},
	}
	// Use server-side apply style: create or update
	existing := &corev1.Secret{}
	err := k8sClient.Get(ctx, types.NamespacedName{Name: name, Namespace: testNamespace}, existing)
	if apierrors.IsNotFound(err) {
		Expect(k8sClient.Create(ctx, secret)).To(Succeed())
	}
	// If it already exists, that's fine
}

func boolPtr(b bool) *bool {
	return &b
}

var _ = Describe("RepositoryMirror", Ordered, func() {
	ctx := context.Background()

	BeforeAll(func() {
		createCredentialsSecret(ctx, "quay-credentials")
	})

	AfterAll(func() {
		// Clean up any leftover CRs
		for _, name := range []string{"test-create", "test-update", "test-delete"} {
			cr := &quayv1alpha1.RepositoryMirror{}
			err := k8sClient.Get(ctx, types.NamespacedName{Name: name, Namespace: testNamespace}, cr)
			if err == nil {
				_ = k8sClient.Delete(ctx, cr)
			}
		}
	})

	It("should create a mirror and reconcile successfully", func() {
		cr := &quayv1alpha1.RepositoryMirror{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-create",
				Namespace: testNamespace,
			},
			Spec: quayv1alpha1.RepositoryMirrorSpec{
				ConnSecretRef: quayv1alpha1.LocalSecretRef{
					Name: "quay-credentials",
				},
				Name:              "testorg/testrepo",
				ExternalReference: "kind-registry:5000/ayoy/quay-config-operator",
				RobotUsername:     "testorg+robot",
				IsEnabled:         boolPtr(true),
				ImageTags:         []string{"latest"},
				SyncInterval:      "86400",
			},
		}
		Expect(k8sClient.Create(ctx, cr)).To(Succeed())

		Eventually(func(g Gomega) {
			var got quayv1alpha1.RepositoryMirror
			g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "test-create", Namespace: testNamespace}, &got)).To(Succeed())
			g.Expect(got.Status.ExistInQuay).To(BeTrue())

			successCond := findCondition(got.Status.Conditions, "Successful")
			g.Expect(successCond).NotTo(BeNil())
			g.Expect(successCond.Status).To(Equal(metav1.ConditionTrue))
		}, "60s", "2s").Should(Succeed())
	})

	It("should update a mirror and reconcile the changes", func() {
		cr := &quayv1alpha1.RepositoryMirror{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-update",
				Namespace: testNamespace,
			},
			Spec: quayv1alpha1.RepositoryMirrorSpec{
				ConnSecretRef: quayv1alpha1.LocalSecretRef{
					Name: "quay-credentials",
				},
				Name:              "testorg/testrepo",
				ExternalReference: "kind-registry:5000/ayoy/quay-config-operator",
				RobotUsername:     "testorg+robot",
				SyncInterval:      "86400",
				ImageTags:         []string{"latest"},
			},
		}
		Expect(k8sClient.Create(ctx, cr)).To(Succeed())

		// Wait for initial reconciliation
		Eventually(func(g Gomega) {
			var got quayv1alpha1.RepositoryMirror
			g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "test-update", Namespace: testNamespace}, &got)).To(Succeed())
			g.Expect(got.Status.ExistInQuay).To(BeTrue())

			successCond := findCondition(got.Status.Conditions, "Successful")
			g.Expect(successCond).NotTo(BeNil())
			g.Expect(successCond.Status).To(Equal(metav1.ConditionTrue))
		}, "60s", "2s").Should(Succeed())

		// Update syncInterval
		var current quayv1alpha1.RepositoryMirror
		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "test-update", Namespace: testNamespace}, &current)).To(Succeed())
		current.Spec.SyncInterval = "172800"
		Expect(k8sClient.Update(ctx, &current)).To(Succeed())

		// Verify still successful after update
		Eventually(func(g Gomega) {
			var got quayv1alpha1.RepositoryMirror
			g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "test-update", Namespace: testNamespace}, &got)).To(Succeed())
			g.Expect(got.Status.ExistInQuay).To(BeTrue())

			successCond := findCondition(got.Status.Conditions, "Successful")
			g.Expect(successCond).NotTo(BeNil())
			g.Expect(successCond.Status).To(Equal(metav1.ConditionTrue))
		}, "60s", "2s").Should(Succeed())
	})

	It("should delete a mirror and clean up", func() {
		cr := &quayv1alpha1.RepositoryMirror{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-delete",
				Namespace: testNamespace,
			},
			Spec: quayv1alpha1.RepositoryMirrorSpec{
				ConnSecretRef: quayv1alpha1.LocalSecretRef{
					Name: "quay-credentials",
				},
				Name:              "testorg/testrepo",
				ExternalReference: "kind-registry:5000/ayoy/quay-config-operator",
				RobotUsername:     "testorg+robot",
			},
		}
		Expect(k8sClient.Create(ctx, cr)).To(Succeed())

		// Wait for it to exist in Quay
		Eventually(func(g Gomega) {
			var got quayv1alpha1.RepositoryMirror
			g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "test-delete", Namespace: testNamespace}, &got)).To(Succeed())
			g.Expect(got.Status.ExistInQuay).To(BeTrue())
		}, "60s", "2s").Should(Succeed())

		// Delete the CR
		toDelete := &quayv1alpha1.RepositoryMirror{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-delete",
				Namespace: testNamespace,
			},
		}
		Expect(k8sClient.Delete(ctx, toDelete)).To(Succeed())

		// Assert CR is gone
		Eventually(func() bool {
			var got quayv1alpha1.RepositoryMirror
			err := k8sClient.Get(ctx, types.NamespacedName{Name: "test-delete", Namespace: testNamespace}, &got)
			return apierrors.IsNotFound(err)
		}, "60s", "2s").Should(BeTrue())
	})
})

func findCondition(conditions []metav1.Condition, condType string) *metav1.Condition {
	for i := range conditions {
		if conditions[i].Type == condType {
			return &conditions[i]
		}
	}
	return nil
}
