/*
Copyright 2024 ayoy.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"

	quayv1alpha1 "github.com/ayoy/quay-config-operator/api/v1alpha1"
	"github.com/ayoy/quay-config-operator/internal/quay"
)

type mockRequest struct {
	Method string
	Path   string
	Body   map[string]interface{}
}

var _ = Describe("RepositoryMirror Controller", func() {
	const (
		mirrorName      = "test-mirror"
		mirrorNamespace = "default"
		secretName      = "quay-credentials"
		repoName        = "myorg/myrepo"
	)

	var (
		reconciler *RepositoryMirrorReconciler
		mockServer *httptest.Server
		mu         sync.Mutex
		requests   []mockRequest
	)

	BeforeEach(func() {
		requests = nil

		mockServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			mu.Lock()
			defer mu.Unlock()

			req := mockRequest{
				Method: r.Method,
				Path:   r.URL.Path,
			}

			if r.Body != nil {
				var body map[string]interface{}
				_ = json.NewDecoder(r.Body).Decode(&body)
				req.Body = body
			}

			requests = append(requests, req)

			switch {
			case r.Method == http.MethodGet && r.URL.Path == "/api/v1/repository/myorg/myrepo/mirror":
				// Return 404 by default (no existing mirror)
				w.WriteHeader(http.StatusNotFound)
			case r.Method == http.MethodPost && r.URL.Path == "/api/v1/repository/myorg/myrepo/mirror":
				w.WriteHeader(http.StatusCreated)
			case r.Method == http.MethodPut && r.URL.Path == "/api/v1/repository/myorg/myrepo/mirror":
				w.WriteHeader(http.StatusOK)
			case r.Method == http.MethodPost && r.URL.Path == "/api/v1/repository/myorg/myrepo/mirror/sync-now":
				w.WriteHeader(http.StatusNoContent)
			default:
				w.WriteHeader(http.StatusNotFound)
			}
		}))

		reconciler = &RepositoryMirrorReconciler{
			Client: k8sClient,
			Scheme: k8sClient.Scheme(),
			QuayClientFunc: func(baseURL, token string, opts ...quay.ClientOption) *quay.Client {
				return quay.NewClient(mockServer.URL, token, opts...)
			},
		}

		// Create the connection secret
		secret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      secretName,
				Namespace: mirrorNamespace,
			},
			Data: map[string][]byte{
				"host":  []byte(mockServer.URL),
				"token": []byte("test-token"),
			},
		}
		err := k8sClient.Create(ctx, secret)
		if err != nil {
			// Secret may already exist from previous test
			_ = k8sClient.Delete(ctx, secret)
			Expect(k8sClient.Create(ctx, secret)).To(Succeed())
		}
	})

	AfterEach(func() {
		mockServer.Close()

		// Cleanup secret
		secret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      secretName,
				Namespace: mirrorNamespace,
			},
		}
		_ = k8sClient.Delete(ctx, secret)
	})

	createMirrorCR := func(name string, spec quayv1alpha1.RepositoryMirrorSpec) *quayv1alpha1.RepositoryMirror {
		mirror := &quayv1alpha1.RepositoryMirror{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: mirrorNamespace,
			},
			Spec: spec,
		}
		Expect(k8sClient.Create(ctx, mirror)).To(Succeed())
		return mirror
	}

	Context("When creating a new RepositoryMirror", func() {
		It("should create the mirror in Quay", func() {
			mirror := createMirrorCR(mirrorName+"-create", quayv1alpha1.RepositoryMirrorSpec{
				ConnSecretRef: quayv1alpha1.LocalSecretRef{
					Name: secretName,
				},
				Name:              repoName,
				ExternalReference: "registry.example.com/repo",
				RobotUsername:     "myorg+robot",
				SyncInterval:      "86400",
			})
			defer func() {
				// Remove finalizer and delete
				Expect(k8sClient.Get(ctx, types.NamespacedName{Name: mirror.Name, Namespace: mirrorNamespace}, mirror)).To(Succeed())
				mirror.Finalizers = nil
				Expect(k8sClient.Update(ctx, mirror)).To(Succeed())
				Expect(k8sClient.Delete(ctx, mirror)).To(Succeed())
			}()

			_, err := reconciler.Reconcile(ctx, ctrl.Request{
				NamespacedName: types.NamespacedName{
					Name:      mirror.Name,
					Namespace: mirrorNamespace,
				},
			})
			Expect(err).NotTo(HaveOccurred())

			// Second reconcile (after finalizer is added) should create the mirror
			_, err = reconciler.Reconcile(ctx, ctrl.Request{
				NamespacedName: types.NamespacedName{
					Name:      mirror.Name,
					Namespace: mirrorNamespace,
				},
			})
			Expect(err).NotTo(HaveOccurred())

			// Verify finalizer was added
			var updated quayv1alpha1.RepositoryMirror
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: mirror.Name, Namespace: mirrorNamespace}, &updated)).To(Succeed())
			Expect(updated.Finalizers).To(ContainElement(finalizerName))

			// Verify a POST request was made
			mu.Lock()
			defer mu.Unlock()
			found := false
			for _, req := range requests {
				if req.Method == http.MethodPost && req.Path == "/api/v1/repository/myorg/myrepo/mirror" {
					found = true
					break
				}
			}
			Expect(found).To(BeTrue(), "expected POST to create mirror")
		})
	})

	Context("When updating an existing RepositoryMirror", func() {
		It("should update the mirror in Quay", func() {
			// Override mock server to return existing mirror
			mockServer.Close()
			mockServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				mu.Lock()
				defer mu.Unlock()

				req := mockRequest{Method: r.Method, Path: r.URL.Path}
				requests = append(requests, req)

				switch {
				case r.Method == http.MethodGet && r.URL.Path == "/api/v1/repository/myorg/myrepo/mirror":
					w.WriteHeader(http.StatusOK)
					_ = json.NewEncoder(w).Encode(quay.MirrorConfig{
						IsEnabled:         true,
						ExternalReference: "registry.example.com/repo",
						SyncInterval:      86400,
					})
				case r.Method == http.MethodPut:
					w.WriteHeader(http.StatusOK)
				default:
					w.WriteHeader(http.StatusNotFound)
				}
			}))

			reconciler.QuayClientFunc = func(baseURL, token string, opts ...quay.ClientOption) *quay.Client {
				return quay.NewClient(mockServer.URL, token, opts...)
			}

			mirror := createMirrorCR(mirrorName+"-update", quayv1alpha1.RepositoryMirrorSpec{
				ConnSecretRef: quayv1alpha1.LocalSecretRef{
					Name: secretName,
				},
				Name:              repoName,
				ExternalReference: "registry.example.com/repo",
				RobotUsername:     "myorg+robot",
				SyncInterval:      "172800",
			})
			defer func() {
				Expect(k8sClient.Get(ctx, types.NamespacedName{Name: mirror.Name, Namespace: mirrorNamespace}, mirror)).To(Succeed())
				mirror.Finalizers = nil
				Expect(k8sClient.Update(ctx, mirror)).To(Succeed())
				Expect(k8sClient.Delete(ctx, mirror)).To(Succeed())
			}()

			// First reconcile adds finalizer
			_, err := reconciler.Reconcile(ctx, ctrl.Request{
				NamespacedName: types.NamespacedName{Name: mirror.Name, Namespace: mirrorNamespace},
			})
			Expect(err).NotTo(HaveOccurred())

			// Second reconcile does the update
			_, err = reconciler.Reconcile(ctx, ctrl.Request{
				NamespacedName: types.NamespacedName{Name: mirror.Name, Namespace: mirrorNamespace},
			})
			Expect(err).NotTo(HaveOccurred())

			// Verify a PUT request was made
			mu.Lock()
			defer mu.Unlock()
			found := false
			for _, req := range requests {
				if req.Method == http.MethodPut {
					found = true
					break
				}
			}
			Expect(found).To(BeTrue(), "expected PUT to update mirror")
		})
	})

	Context("When deleting a RepositoryMirror", func() {
		It("should disable the mirror in Quay and remove finalizer", func() {
			// Override mock server
			mockServer.Close()
			mockServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				mu.Lock()
				defer mu.Unlock()

				req := mockRequest{Method: r.Method, Path: r.URL.Path}
				if r.Body != nil {
					var body map[string]interface{}
					_ = json.NewDecoder(r.Body).Decode(&body)
					req.Body = body
				}
				requests = append(requests, req)

				switch {
				case r.Method == http.MethodGet && r.URL.Path == "/api/v1/repository/myorg/myrepo/mirror":
					w.WriteHeader(http.StatusNotFound)
				case r.Method == http.MethodPost && r.URL.Path == "/api/v1/repository/myorg/myrepo/mirror":
					w.WriteHeader(http.StatusCreated)
				case r.Method == http.MethodPut:
					w.WriteHeader(http.StatusOK)
				default:
					w.WriteHeader(http.StatusNotFound)
				}
			}))

			reconciler.QuayClientFunc = func(baseURL, token string, opts ...quay.ClientOption) *quay.Client {
				return quay.NewClient(mockServer.URL, token, opts...)
			}

			mirror := createMirrorCR(mirrorName+"-delete", quayv1alpha1.RepositoryMirrorSpec{
				ConnSecretRef: quayv1alpha1.LocalSecretRef{
					Name: secretName,
				},
				Name:              repoName,
				ExternalReference: "registry.example.com/repo",
				RobotUsername:     "myorg+robot",
			})

			// Reconcile to add finalizer and create mirror
			_, err := reconciler.Reconcile(ctx, ctrl.Request{
				NamespacedName: types.NamespacedName{Name: mirror.Name, Namespace: mirrorNamespace},
			})
			Expect(err).NotTo(HaveOccurred())
			_, err = reconciler.Reconcile(ctx, ctrl.Request{
				NamespacedName: types.NamespacedName{Name: mirror.Name, Namespace: mirrorNamespace},
			})
			Expect(err).NotTo(HaveOccurred())

			// Delete the CR
			Expect(k8sClient.Delete(ctx, mirror)).To(Succeed())

			// Reset requests to track deletion behavior
			mu.Lock()
			requests = nil
			mu.Unlock()

			// Reconcile after delete
			_, err = reconciler.Reconcile(ctx, ctrl.Request{
				NamespacedName: types.NamespacedName{Name: mirror.Name, Namespace: mirrorNamespace},
			})
			Expect(err).NotTo(HaveOccurred())

			// Verify a PUT request was made to disable the mirror
			mu.Lock()
			defer mu.Unlock()
			found := false
			for _, req := range requests {
				if req.Method == http.MethodPut {
					found = true
					if req.Body != nil {
						if isEnabled, ok := req.Body["is_enabled"]; ok {
							Expect(isEnabled).To(BeFalse(), "expected is_enabled to be false")
						}
					}
					break
				}
			}
			Expect(found).To(BeTrue(), "expected PUT to disable mirror")
		})

		It("should not call Quay API when preserveInQuayOnDeletion is true", func() {
			// Override mock server
			mockServer.Close()
			mockServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				mu.Lock()
				defer mu.Unlock()

				req := mockRequest{Method: r.Method, Path: r.URL.Path}
				requests = append(requests, req)

				switch {
				case r.Method == http.MethodGet && r.URL.Path == "/api/v1/repository/myorg/myrepo/mirror":
					w.WriteHeader(http.StatusNotFound)
				case r.Method == http.MethodPost && r.URL.Path == "/api/v1/repository/myorg/myrepo/mirror":
					w.WriteHeader(http.StatusCreated)
				default:
					w.WriteHeader(http.StatusOK)
				}
			}))

			reconciler.QuayClientFunc = func(baseURL, token string, opts ...quay.ClientOption) *quay.Client {
				return quay.NewClient(mockServer.URL, token, opts...)
			}

			preserve := true
			mirror := createMirrorCR(mirrorName+"-preserve", quayv1alpha1.RepositoryMirrorSpec{
				ConnSecretRef: quayv1alpha1.LocalSecretRef{
					Name: secretName,
				},
				Name:                     repoName,
				ExternalReference:        "registry.example.com/repo",
				RobotUsername:            "myorg+robot",
				PreserveInQuayOnDeletion: &preserve,
			})

			// Reconcile to add finalizer
			_, err := reconciler.Reconcile(ctx, ctrl.Request{
				NamespacedName: types.NamespacedName{Name: mirror.Name, Namespace: mirrorNamespace},
			})
			Expect(err).NotTo(HaveOccurred())
			_, err = reconciler.Reconcile(ctx, ctrl.Request{
				NamespacedName: types.NamespacedName{Name: mirror.Name, Namespace: mirrorNamespace},
			})
			Expect(err).NotTo(HaveOccurred())

			// Delete the CR
			Expect(k8sClient.Delete(ctx, mirror)).To(Succeed())

			// Reset requests
			mu.Lock()
			requests = nil
			mu.Unlock()

			// Reconcile after delete
			_, err = reconciler.Reconcile(ctx, ctrl.Request{
				NamespacedName: types.NamespacedName{Name: mirror.Name, Namespace: mirrorNamespace},
			})
			Expect(err).NotTo(HaveOccurred())

			// Verify NO PUT request was made
			mu.Lock()
			defer mu.Unlock()
			for _, req := range requests {
				Expect(req.Method).NotTo(Equal(http.MethodPut), "expected no PUT request when preserveInQuayOnDeletion is true")
			}
		})
	})

	Context("When the connection secret is not found", func() {
		It("should set error condition", func() {
			mirror := createMirrorCR(mirrorName+"-nosecret", quayv1alpha1.RepositoryMirrorSpec{
				ConnSecretRef: quayv1alpha1.LocalSecretRef{
					Name: "nonexistent-secret",
				},
				Name:              repoName,
				ExternalReference: "registry.example.com/repo",
				RobotUsername:     "myorg+robot",
			})
			defer func() {
				Expect(k8sClient.Get(ctx, types.NamespacedName{Name: mirror.Name, Namespace: mirrorNamespace}, mirror)).To(Succeed())
				mirror.Finalizers = nil
				Expect(k8sClient.Update(ctx, mirror)).To(Succeed())
				Expect(k8sClient.Delete(ctx, mirror)).To(Succeed())
			}()

			_, err := reconciler.Reconcile(ctx, ctrl.Request{
				NamespacedName: types.NamespacedName{Name: mirror.Name, Namespace: mirrorNamespace},
			})
			Expect(err).NotTo(HaveOccurred()) // Error is captured in status, not returned

			var updated quayv1alpha1.RepositoryMirror
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: mirror.Name, Namespace: mirrorNamespace}, &updated)).To(Succeed())

			// Check that the Successful condition is False
			found := false
			for _, c := range updated.Status.Conditions {
				if c.Type == conditionSuccess {
					found = true
					Expect(c.Status).To(Equal(metav1.ConditionFalse))
					Expect(c.Reason).To(Equal("SecretError"))
				}
			}
			Expect(found).To(BeTrue(), "expected Successful condition to be set")
		})
	})

	Context("When the repository name is invalid", func() {
		It("should be rejected by CRD validation", func() {
			mirror := &quayv1alpha1.RepositoryMirror{
				ObjectMeta: metav1.ObjectMeta{
					Name:      mirrorName + "-invalidname",
					Namespace: mirrorNamespace,
				},
				Spec: quayv1alpha1.RepositoryMirrorSpec{
					ConnSecretRef: quayv1alpha1.LocalSecretRef{
						Name: secretName,
					},
					Name:              "invalid-no-slash",
					ExternalReference: "registry.example.com/repo",
					RobotUsername:     "myorg+robot",
				},
			}
			err := k8sClient.Create(ctx, mirror)
			Expect(err).To(HaveOccurred(), "expected CRD validation to reject invalid name")
		})
	})

	Context("When forceSync is enabled", func() {
		It("should trigger SyncNow", func() {
			// Override mock server to return existing mirror
			mockServer.Close()
			mockServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				mu.Lock()
				defer mu.Unlock()

				req := mockRequest{Method: r.Method, Path: r.URL.Path}
				requests = append(requests, req)

				switch {
				case r.Method == http.MethodGet && r.URL.Path == "/api/v1/repository/myorg/myrepo/mirror":
					w.WriteHeader(http.StatusNotFound)
				case r.Method == http.MethodPost && r.URL.Path == "/api/v1/repository/myorg/myrepo/mirror":
					w.WriteHeader(http.StatusCreated)
				case r.Method == http.MethodPost && r.URL.Path == "/api/v1/repository/myorg/myrepo/mirror/sync-now":
					w.WriteHeader(http.StatusNoContent)
				default:
					w.WriteHeader(http.StatusOK)
				}
			}))

			reconciler.QuayClientFunc = func(baseURL, token string, opts ...quay.ClientOption) *quay.Client {
				return quay.NewClient(mockServer.URL, token, opts...)
			}

			forceSync := true
			mirror := createMirrorCR(mirrorName+"-sync", quayv1alpha1.RepositoryMirrorSpec{
				ConnSecretRef: quayv1alpha1.LocalSecretRef{
					Name: secretName,
				},
				Name:              repoName,
				ExternalReference: "registry.example.com/repo",
				RobotUsername:     "myorg+robot",
				ForceSync:         &forceSync,
			})
			defer func() {
				Expect(k8sClient.Get(ctx, types.NamespacedName{Name: mirror.Name, Namespace: mirrorNamespace}, mirror)).To(Succeed())
				mirror.Finalizers = nil
				Expect(k8sClient.Update(ctx, mirror)).To(Succeed())
				Expect(k8sClient.Delete(ctx, mirror)).To(Succeed())
			}()

			// First reconcile adds finalizer
			_, err := reconciler.Reconcile(ctx, ctrl.Request{
				NamespacedName: types.NamespacedName{Name: mirror.Name, Namespace: mirrorNamespace},
			})
			Expect(err).NotTo(HaveOccurred())

			// Second reconcile creates mirror + triggers sync
			_, err = reconciler.Reconcile(ctx, ctrl.Request{
				NamespacedName: types.NamespacedName{Name: mirror.Name, Namespace: mirrorNamespace},
			})
			Expect(err).NotTo(HaveOccurred())

			// Verify sync-now request was made
			mu.Lock()
			defer mu.Unlock()
			found := false
			for _, req := range requests {
				if req.Method == http.MethodPost && req.Path == "/api/v1/repository/myorg/myrepo/mirror/sync-now" {
					found = true
					break
				}
			}
			Expect(found).To(BeTrue(), "expected POST to sync-now endpoint")
		})
	})

	Context("When GetMirror API fails", func() {
		It("should set QuayAPIError condition", func() {
			mockServer.Close()
			mockServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodGet && r.URL.Path == "/api/v1/repository/myorg/myrepo/mirror" {
					w.WriteHeader(http.StatusInternalServerError)
				} else {
					w.WriteHeader(http.StatusNotFound)
				}
			}))

			reconciler.QuayClientFunc = func(baseURL, token string, opts ...quay.ClientOption) *quay.Client {
				return quay.NewClient(mockServer.URL, token, opts...)
			}

			mirror := createMirrorCR(mirrorName+"-getfail", quayv1alpha1.RepositoryMirrorSpec{
				ConnSecretRef: quayv1alpha1.LocalSecretRef{
					Name: secretName,
				},
				Name:              repoName,
				ExternalReference: "registry.example.com/repo",
				RobotUsername:     "myorg+robot",
			})
			defer func() {
				Expect(k8sClient.Get(ctx, types.NamespacedName{Name: mirror.Name, Namespace: mirrorNamespace}, mirror)).To(Succeed())
				mirror.Finalizers = nil
				Expect(k8sClient.Update(ctx, mirror)).To(Succeed())
				Expect(k8sClient.Delete(ctx, mirror)).To(Succeed())
			}()

			// First reconcile adds finalizer
			_, err := reconciler.Reconcile(ctx, ctrl.Request{
				NamespacedName: types.NamespacedName{Name: mirror.Name, Namespace: mirrorNamespace},
			})
			Expect(err).NotTo(HaveOccurred())

			// Second reconcile hits GetMirror error
			_, err = reconciler.Reconcile(ctx, ctrl.Request{
				NamespacedName: types.NamespacedName{Name: mirror.Name, Namespace: mirrorNamespace},
			})
			Expect(err).NotTo(HaveOccurred())

			var updated quayv1alpha1.RepositoryMirror
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: mirror.Name, Namespace: mirrorNamespace}, &updated)).To(Succeed())

			found := false
			for _, c := range updated.Status.Conditions {
				if c.Type == conditionSuccess {
					found = true
					Expect(c.Status).To(Equal(metav1.ConditionFalse))
					Expect(c.Reason).To(Equal("QuayAPIError"))
				}
			}
			Expect(found).To(BeTrue(), "expected Successful condition to be set")
		})
	})

	Context("When CreateMirror API fails", func() {
		It("should set CreateError condition", func() {
			mockServer.Close()
			mockServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch {
				case r.Method == http.MethodGet && r.URL.Path == "/api/v1/repository/myorg/myrepo/mirror":
					w.WriteHeader(http.StatusNotFound)
				case r.Method == http.MethodPost && r.URL.Path == "/api/v1/repository/myorg/myrepo/mirror":
					w.WriteHeader(http.StatusInternalServerError)
				default:
					w.WriteHeader(http.StatusNotFound)
				}
			}))

			reconciler.QuayClientFunc = func(baseURL, token string, opts ...quay.ClientOption) *quay.Client {
				return quay.NewClient(mockServer.URL, token, opts...)
			}

			mirror := createMirrorCR(mirrorName+"-createfail", quayv1alpha1.RepositoryMirrorSpec{
				ConnSecretRef: quayv1alpha1.LocalSecretRef{
					Name: secretName,
				},
				Name:              repoName,
				ExternalReference: "registry.example.com/repo",
				RobotUsername:     "myorg+robot",
			})
			defer func() {
				Expect(k8sClient.Get(ctx, types.NamespacedName{Name: mirror.Name, Namespace: mirrorNamespace}, mirror)).To(Succeed())
				mirror.Finalizers = nil
				Expect(k8sClient.Update(ctx, mirror)).To(Succeed())
				Expect(k8sClient.Delete(ctx, mirror)).To(Succeed())
			}()

			// First reconcile adds finalizer
			_, err := reconciler.Reconcile(ctx, ctrl.Request{
				NamespacedName: types.NamespacedName{Name: mirror.Name, Namespace: mirrorNamespace},
			})
			Expect(err).NotTo(HaveOccurred())

			// Second reconcile hits CreateMirror error
			_, err = reconciler.Reconcile(ctx, ctrl.Request{
				NamespacedName: types.NamespacedName{Name: mirror.Name, Namespace: mirrorNamespace},
			})
			Expect(err).NotTo(HaveOccurred())

			var updated quayv1alpha1.RepositoryMirror
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: mirror.Name, Namespace: mirrorNamespace}, &updated)).To(Succeed())

			found := false
			for _, c := range updated.Status.Conditions {
				if c.Type == conditionSuccess {
					found = true
					Expect(c.Status).To(Equal(metav1.ConditionFalse))
					Expect(c.Reason).To(Equal("CreateError"))
				}
			}
			Expect(found).To(BeTrue(), "expected Successful condition to be set")
		})
	})

	Context("When UpdateMirror API fails", func() {
		It("should set UpdateError condition", func() {
			mockServer.Close()
			mockServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch {
				case r.Method == http.MethodGet && r.URL.Path == "/api/v1/repository/myorg/myrepo/mirror":
					w.WriteHeader(http.StatusOK)
					_ = json.NewEncoder(w).Encode(quay.MirrorConfig{
						IsEnabled:         true,
						ExternalReference: "registry.example.com/repo",
						SyncInterval:      86400,
					})
				case r.Method == http.MethodPut && r.URL.Path == "/api/v1/repository/myorg/myrepo/mirror":
					w.WriteHeader(http.StatusInternalServerError)
				default:
					w.WriteHeader(http.StatusNotFound)
				}
			}))

			reconciler.QuayClientFunc = func(baseURL, token string, opts ...quay.ClientOption) *quay.Client {
				return quay.NewClient(mockServer.URL, token, opts...)
			}

			mirror := createMirrorCR(mirrorName+"-updatefail", quayv1alpha1.RepositoryMirrorSpec{
				ConnSecretRef: quayv1alpha1.LocalSecretRef{
					Name: secretName,
				},
				Name:              repoName,
				ExternalReference: "registry.example.com/repo",
				RobotUsername:     "myorg+robot",
			})
			defer func() {
				Expect(k8sClient.Get(ctx, types.NamespacedName{Name: mirror.Name, Namespace: mirrorNamespace}, mirror)).To(Succeed())
				mirror.Finalizers = nil
				Expect(k8sClient.Update(ctx, mirror)).To(Succeed())
				Expect(k8sClient.Delete(ctx, mirror)).To(Succeed())
			}()

			// First reconcile adds finalizer
			_, err := reconciler.Reconcile(ctx, ctrl.Request{
				NamespacedName: types.NamespacedName{Name: mirror.Name, Namespace: mirrorNamespace},
			})
			Expect(err).NotTo(HaveOccurred())

			// Second reconcile hits UpdateMirror error
			_, err = reconciler.Reconcile(ctx, ctrl.Request{
				NamespacedName: types.NamespacedName{Name: mirror.Name, Namespace: mirrorNamespace},
			})
			Expect(err).NotTo(HaveOccurred())

			var updated quayv1alpha1.RepositoryMirror
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: mirror.Name, Namespace: mirrorNamespace}, &updated)).To(Succeed())

			found := false
			for _, c := range updated.Status.Conditions {
				if c.Type == conditionSuccess {
					found = true
					Expect(c.Status).To(Equal(metav1.ConditionFalse))
					Expect(c.Reason).To(Equal("UpdateError"))
				}
			}
			Expect(found).To(BeTrue(), "expected Successful condition to be set")
		})
	})

	Context("When SyncNow API fails", func() {
		It("should set SyncError condition", func() {
			mockServer.Close()
			mockServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch {
				case r.Method == http.MethodGet && r.URL.Path == "/api/v1/repository/myorg/myrepo/mirror":
					w.WriteHeader(http.StatusNotFound)
				case r.Method == http.MethodPost && r.URL.Path == "/api/v1/repository/myorg/myrepo/mirror":
					w.WriteHeader(http.StatusCreated)
				case r.Method == http.MethodPost && r.URL.Path == "/api/v1/repository/myorg/myrepo/mirror/sync-now":
					w.WriteHeader(http.StatusInternalServerError)
				default:
					w.WriteHeader(http.StatusNotFound)
				}
			}))

			reconciler.QuayClientFunc = func(baseURL, token string, opts ...quay.ClientOption) *quay.Client {
				return quay.NewClient(mockServer.URL, token, opts...)
			}

			forceSync := true
			mirror := createMirrorCR(mirrorName+"-syncfail", quayv1alpha1.RepositoryMirrorSpec{
				ConnSecretRef: quayv1alpha1.LocalSecretRef{
					Name: secretName,
				},
				Name:              repoName,
				ExternalReference: "registry.example.com/repo",
				RobotUsername:     "myorg+robot",
				ForceSync:         &forceSync,
			})
			defer func() {
				Expect(k8sClient.Get(ctx, types.NamespacedName{Name: mirror.Name, Namespace: mirrorNamespace}, mirror)).To(Succeed())
				mirror.Finalizers = nil
				Expect(k8sClient.Update(ctx, mirror)).To(Succeed())
				Expect(k8sClient.Delete(ctx, mirror)).To(Succeed())
			}()

			// First reconcile adds finalizer
			_, err := reconciler.Reconcile(ctx, ctrl.Request{
				NamespacedName: types.NamespacedName{Name: mirror.Name, Namespace: mirrorNamespace},
			})
			Expect(err).NotTo(HaveOccurred())

			// Second reconcile creates mirror then hits SyncNow error
			_, err = reconciler.Reconcile(ctx, ctrl.Request{
				NamespacedName: types.NamespacedName{Name: mirror.Name, Namespace: mirrorNamespace},
			})
			Expect(err).NotTo(HaveOccurred())

			var updated quayv1alpha1.RepositoryMirror
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: mirror.Name, Namespace: mirrorNamespace}, &updated)).To(Succeed())

			found := false
			for _, c := range updated.Status.Conditions {
				if c.Type == conditionSuccess {
					found = true
					Expect(c.Status).To(Equal(metav1.ConditionFalse))
					Expect(c.Reason).To(Equal("SyncError"))
				}
			}
			Expect(found).To(BeTrue(), "expected Successful condition to be set")
		})
	})

	Context("When externalReference contains scheme", func() {
		It("should set InvalidSpec condition", func() {
			mirror := createMirrorCR(mirrorName+"-invalidextref", quayv1alpha1.RepositoryMirrorSpec{
				ConnSecretRef: quayv1alpha1.LocalSecretRef{
					Name: secretName,
				},
				Name:              repoName,
				ExternalReference: "file://etc/passwd",
				RobotUsername:     "myorg+robot",
			})
			defer func() {
				Expect(k8sClient.Get(ctx, types.NamespacedName{Name: mirror.Name, Namespace: mirrorNamespace}, mirror)).To(Succeed())
				mirror.Finalizers = nil
				Expect(k8sClient.Update(ctx, mirror)).To(Succeed())
				Expect(k8sClient.Delete(ctx, mirror)).To(Succeed())
			}()

			// First reconcile adds finalizer
			_, err := reconciler.Reconcile(ctx, ctrl.Request{
				NamespacedName: types.NamespacedName{Name: mirror.Name, Namespace: mirrorNamespace},
			})
			Expect(err).NotTo(HaveOccurred())

			// Second reconcile hits validateSpec failure
			_, err = reconciler.Reconcile(ctx, ctrl.Request{
				NamespacedName: types.NamespacedName{Name: mirror.Name, Namespace: mirrorNamespace},
			})
			Expect(err).NotTo(HaveOccurred())

			var updated quayv1alpha1.RepositoryMirror
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: mirror.Name, Namespace: mirrorNamespace}, &updated)).To(Succeed())

			found := false
			for _, c := range updated.Status.Conditions {
				if c.Type == conditionSuccess {
					found = true
					Expect(c.Status).To(Equal(metav1.ConditionFalse))
					Expect(c.Reason).To(Equal("InvalidSpec"))
				}
			}
			Expect(found).To(BeTrue(), "expected Successful condition to be set")
		})
	})

	Context("When external registry credentials are provided", func() {
		It("should use credentials from secret", func() {
			// Create external credentials secret
			extSecret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "ext-creds",
					Namespace: mirrorNamespace,
				},
				Data: map[string][]byte{
					"username": []byte("ext-user"),
					"password": []byte("ext-pass"),
				},
			}
			err := k8sClient.Create(ctx, extSecret)
			if err != nil {
				_ = k8sClient.Delete(ctx, extSecret)
				Expect(k8sClient.Create(ctx, extSecret)).To(Succeed())
			}
			defer func() {
				_ = k8sClient.Delete(ctx, extSecret)
			}()

			mockServer.Close()
			var capturedBody map[string]interface{}
			mockServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				mu.Lock()
				defer mu.Unlock()

				switch {
				case r.Method == http.MethodGet && r.URL.Path == "/api/v1/repository/myorg/myrepo/mirror":
					w.WriteHeader(http.StatusNotFound)
				case r.Method == http.MethodPost && r.URL.Path == "/api/v1/repository/myorg/myrepo/mirror":
					var body map[string]interface{}
					_ = json.NewDecoder(r.Body).Decode(&body)
					capturedBody = body
					w.WriteHeader(http.StatusCreated)
				default:
					w.WriteHeader(http.StatusNotFound)
				}
			}))

			reconciler.QuayClientFunc = func(baseURL, token string, opts ...quay.ClientOption) *quay.Client {
				return quay.NewClient(mockServer.URL, token, opts...)
			}

			mirror := createMirrorCR(mirrorName+"-extcreds", quayv1alpha1.RepositoryMirrorSpec{
				ConnSecretRef: quayv1alpha1.LocalSecretRef{
					Name: secretName,
				},
				Name:              repoName,
				ExternalReference: "registry.example.com/repo",
				RobotUsername:     "myorg+robot",
				ExternalRegistryCredentialsRef: &quayv1alpha1.LocalSecretRef{
					Name: "ext-creds",
				},
			})
			defer func() {
				Expect(k8sClient.Get(ctx, types.NamespacedName{Name: mirror.Name, Namespace: mirrorNamespace}, mirror)).To(Succeed())
				mirror.Finalizers = nil
				Expect(k8sClient.Update(ctx, mirror)).To(Succeed())
				Expect(k8sClient.Delete(ctx, mirror)).To(Succeed())
			}()

			// First reconcile adds finalizer
			_, err = reconciler.Reconcile(ctx, ctrl.Request{
				NamespacedName: types.NamespacedName{Name: mirror.Name, Namespace: mirrorNamespace},
			})
			Expect(err).NotTo(HaveOccurred())

			// Second reconcile creates mirror with credentials
			_, err = reconciler.Reconcile(ctx, ctrl.Request{
				NamespacedName: types.NamespacedName{Name: mirror.Name, Namespace: mirrorNamespace},
			})
			Expect(err).NotTo(HaveOccurred())

			mu.Lock()
			defer mu.Unlock()
			Expect(capturedBody).NotTo(BeNil())
			Expect(capturedBody["external_registry_username"]).To(Equal("ext-user"))
			Expect(capturedBody["external_registry_password"]).To(Equal("ext-pass"))
		})
	})

	Context("When external registry credentials secret is missing", func() {
		It("should set CreateError condition", func() {
			mockServer.Close()
			mockServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodGet && r.URL.Path == "/api/v1/repository/myorg/myrepo/mirror" {
					w.WriteHeader(http.StatusNotFound)
				} else {
					w.WriteHeader(http.StatusNotFound)
				}
			}))

			reconciler.QuayClientFunc = func(baseURL, token string, opts ...quay.ClientOption) *quay.Client {
				return quay.NewClient(mockServer.URL, token, opts...)
			}

			mirror := createMirrorCR(mirrorName+"-missingextcreds", quayv1alpha1.RepositoryMirrorSpec{
				ConnSecretRef: quayv1alpha1.LocalSecretRef{
					Name: secretName,
				},
				Name:              repoName,
				ExternalReference: "registry.example.com/repo",
				RobotUsername:     "myorg+robot",
				ExternalRegistryCredentialsRef: &quayv1alpha1.LocalSecretRef{
					Name: "nonexistent-ext-creds",
				},
			})
			defer func() {
				Expect(k8sClient.Get(ctx, types.NamespacedName{Name: mirror.Name, Namespace: mirrorNamespace}, mirror)).To(Succeed())
				mirror.Finalizers = nil
				Expect(k8sClient.Update(ctx, mirror)).To(Succeed())
				Expect(k8sClient.Delete(ctx, mirror)).To(Succeed())
			}()

			// First reconcile adds finalizer
			_, err := reconciler.Reconcile(ctx, ctrl.Request{
				NamespacedName: types.NamespacedName{Name: mirror.Name, Namespace: mirrorNamespace},
			})
			Expect(err).NotTo(HaveOccurred())

			// Second reconcile hits missing credentials secret
			_, err = reconciler.Reconcile(ctx, ctrl.Request{
				NamespacedName: types.NamespacedName{Name: mirror.Name, Namespace: mirrorNamespace},
			})
			Expect(err).NotTo(HaveOccurred())

			var updated quayv1alpha1.RepositoryMirror
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: mirror.Name, Namespace: mirrorNamespace}, &updated)).To(Succeed())

			found := false
			for _, c := range updated.Status.Conditions {
				if c.Type == conditionSuccess {
					found = true
					Expect(c.Status).To(Equal(metav1.ConditionFalse))
					Expect(c.Reason).To(Equal("CreateError"))
				}
			}
			Expect(found).To(BeTrue(), "expected Successful condition to be set")
		})
	})

	Context("When proxy and TLS settings are configured", func() {
		It("should include external registry config in request", func() {
			mockServer.Close()
			var capturedBody map[string]interface{}
			mockServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				mu.Lock()
				defer mu.Unlock()

				switch {
				case r.Method == http.MethodGet && r.URL.Path == "/api/v1/repository/myorg/myrepo/mirror":
					w.WriteHeader(http.StatusNotFound)
				case r.Method == http.MethodPost && r.URL.Path == "/api/v1/repository/myorg/myrepo/mirror":
					var body map[string]interface{}
					_ = json.NewDecoder(r.Body).Decode(&body)
					capturedBody = body
					w.WriteHeader(http.StatusCreated)
				default:
					w.WriteHeader(http.StatusNotFound)
				}
			}))

			reconciler.QuayClientFunc = func(baseURL, token string, opts ...quay.ClientOption) *quay.Client {
				return quay.NewClient(mockServer.URL, token, opts...)
			}

			verifyTls := false
			unsignedImages := true
			mirror := createMirrorCR(mirrorName+"-proxyconfig", quayv1alpha1.RepositoryMirrorSpec{
				ConnSecretRef: quayv1alpha1.LocalSecretRef{
					Name: secretName,
				},
				Name:              repoName,
				ExternalReference: "registry.example.com/repo",
				RobotUsername:     "myorg+robot",
				VerifyTls:         &verifyTls,
				UnsignedImages:    &unsignedImages,
				HttpProxy:         "http://proxy:8080",
				HttpsProxy:        "https://proxy:8443",
				NoProxy:           "localhost",
			})
			defer func() {
				Expect(k8sClient.Get(ctx, types.NamespacedName{Name: mirror.Name, Namespace: mirrorNamespace}, mirror)).To(Succeed())
				mirror.Finalizers = nil
				Expect(k8sClient.Update(ctx, mirror)).To(Succeed())
				Expect(k8sClient.Delete(ctx, mirror)).To(Succeed())
			}()

			// First reconcile adds finalizer
			_, err := reconciler.Reconcile(ctx, ctrl.Request{
				NamespacedName: types.NamespacedName{Name: mirror.Name, Namespace: mirrorNamespace},
			})
			Expect(err).NotTo(HaveOccurred())

			// Second reconcile creates mirror with proxy config
			_, err = reconciler.Reconcile(ctx, ctrl.Request{
				NamespacedName: types.NamespacedName{Name: mirror.Name, Namespace: mirrorNamespace},
			})
			Expect(err).NotTo(HaveOccurred())

			mu.Lock()
			defer mu.Unlock()
			Expect(capturedBody).NotTo(BeNil())
			extConfig, ok := capturedBody["external_registry_config"].(map[string]interface{})
			Expect(ok).To(BeTrue())
			Expect(extConfig["verify_tls"]).To(BeFalse())
			Expect(extConfig["unsigned_images"]).To(BeTrue())
			proxy, ok := extConfig["proxy"].(map[string]interface{})
			Expect(ok).To(BeTrue())
			Expect(proxy["http_proxy"]).To(Equal("http://proxy:8080"))
			Expect(proxy["https_proxy"]).To(Equal("https://proxy:8443"))
			Expect(proxy["no_proxy"]).To(Equal("localhost"))
		})
	})
})

// Standalone unit tests using testing.T

func TestValidateProxyURL(t *testing.T) {
	tests := []struct {
		name      string
		proxyURL  string
		fieldName string
		wantErr   bool
	}{
		{
			name:      "empty URL is valid",
			proxyURL:  "",
			fieldName: "httpProxy",
			wantErr:   false,
		},
		{
			name:      "http scheme is valid",
			proxyURL:  "http://proxy:8080",
			fieldName: "httpProxy",
			wantErr:   false,
		},
		{
			name:      "https scheme is valid",
			proxyURL:  "https://proxy:8443",
			fieldName: "httpsProxy",
			wantErr:   false,
		},
		{
			name:      "ftp scheme is invalid",
			proxyURL:  "ftp://proxy:21",
			fieldName: "httpProxy",
			wantErr:   true,
		},
		{
			name:      "just-a-string without scheme is invalid",
			proxyURL:  "just-a-string",
			fieldName: "httpProxy",
			wantErr:   true,
		},
		{
			name:      "missing host is invalid",
			proxyURL:  "http://",
			fieldName: "httpProxy",
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateProxyURL(tt.proxyURL, tt.fieldName)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateProxyURL() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateSpec(t *testing.T) {
	tests := []struct {
		name    string
		spec    quayv1alpha1.RepositoryMirrorSpec
		wantErr bool
	}{
		{
			name: "empty spec is valid",
			spec: quayv1alpha1.RepositoryMirrorSpec{
				Name: "org/repo",
			},
			wantErr: false,
		},
		{
			name: "externalReference without scheme is valid",
			spec: quayv1alpha1.RepositoryMirrorSpec{
				Name:              "org/repo",
				ExternalReference: "registry.example.com/repo",
			},
			wantErr: false,
		},
		{
			name: "externalReference with file scheme is invalid",
			spec: quayv1alpha1.RepositoryMirrorSpec{
				Name:              "org/repo",
				ExternalReference: "file://etc/passwd",
			},
			wantErr: true,
		},
		{
			name: "externalReference with http scheme is invalid",
			spec: quayv1alpha1.RepositoryMirrorSpec{
				Name:              "org/repo",
				ExternalReference: "http://registry.example.com",
			},
			wantErr: true,
		},
		{
			name: "httpProxy with ftp scheme is invalid",
			spec: quayv1alpha1.RepositoryMirrorSpec{
				Name:      "org/repo",
				HttpProxy: "ftp://bad",
			},
			wantErr: true,
		},
		{
			name: "httpsProxy with ftp scheme is invalid",
			spec: quayv1alpha1.RepositoryMirrorSpec{
				Name:       "org/repo",
				HttpsProxy: "ftp://bad",
			},
			wantErr: true,
		},
		{
			name: "valid httpProxy and httpsProxy",
			spec: quayv1alpha1.RepositoryMirrorSpec{
				Name:       "org/repo",
				HttpProxy:  "http://proxy:8080",
				HttpsProxy: "https://proxy:8443",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateSpec(&tt.spec)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateSpec() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestParseRepoName(t *testing.T) {
	tests := []struct {
		name          string
		repoName      string
		wantNamespace string
		wantRepo      string
		wantErr       bool
	}{
		{
			name:          "valid org/repo",
			repoName:      "org/repo",
			wantNamespace: "org",
			wantRepo:      "repo",
			wantErr:       false,
		},
		{
			name:          "valid my-org/my-repo with hyphens",
			repoName:      "my-org/my-repo",
			wantNamespace: "my-org",
			wantRepo:      "my-repo",
			wantErr:       false,
		},
		{
			name:          "valid a.b/c.d with dots",
			repoName:      "a.b/c.d",
			wantNamespace: "a.b",
			wantRepo:      "c.d",
			wantErr:       false,
		},
		{
			name:          "noslash is invalid",
			repoName:      "noslash",
			wantNamespace: "",
			wantRepo:      "",
			wantErr:       true,
		},
		{
			name:          "/repo with missing namespace is invalid",
			repoName:      "/repo",
			wantNamespace: "",
			wantRepo:      "",
			wantErr:       true,
		},
		{
			name:          "org/ with missing repo is invalid",
			repoName:      "org/",
			wantNamespace: "",
			wantRepo:      "",
			wantErr:       true,
		},
		{
			name:          "org/repo- with trailing hyphen is invalid",
			repoName:      "org/repo-",
			wantNamespace: "",
			wantRepo:      "",
			wantErr:       true,
		},
		{
			name:          "org/a..b with consecutive dots is invalid",
			repoName:      "org/a..b",
			wantNamespace: "",
			wantRepo:      "",
			wantErr:       true,
		},
		{
			name:          "org-/repo with trailing hyphen in namespace is invalid",
			repoName:      "org-/repo",
			wantNamespace: "",
			wantRepo:      "",
			wantErr:       true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotNamespace, gotRepo, err := parseRepoName(tt.repoName)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseRepoName() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if gotNamespace != tt.wantNamespace {
				t.Errorf("parseRepoName() gotNamespace = %v, want %v", gotNamespace, tt.wantNamespace)
			}
			if gotRepo != tt.wantRepo {
				t.Errorf("parseRepoName() gotRepo = %v, want %v", gotRepo, tt.wantRepo)
			}
		})
	}
}
