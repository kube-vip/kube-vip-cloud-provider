//go:build e2e

package e2e

import (
	"context"
	"os"
	"path/filepath"
	"time"

	"github.com/onsi/ginkgo/v2"
	"github.com/stretchr/testify/require"
	core_v1 "k8s.io/api/core/v1"
	api_errors "k8s.io/apimachinery/pkg/api/errors"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

// Framework provides a collection of helpful functions for
// writing end-to-end (E2E) tests for Kube-vip-cloud-provider.
type Framework struct {
	// Client is a client-go Kubernetes client.
	Client kubernetes.Interface

	// RetryInterval is how often to retry polling operations.
	RetryInterval time.Duration

	// RetryTimeout is how long to continue trying polling
	// operations before giving up.
	RetryTimeout time.Duration

	// Deployment provides helpers for managing deploying resources that
	// are part of a full Kube-vip-cloud-provider deployment manifest.
	Deployment *Deployment

	t ginkgo.GinkgoTInterface
}

func NewFramework() *Framework {
	t := ginkgo.GinkgoT()

	// Deferring GinkgoRecover() provides better error messages in case of panic
	// e.g. when KUBE_VIP_CLOUD_PROVIDER_E2E_LOCAL_HOST environment variable is not set.
	defer ginkgo.GinkgoRecover()

	var (
		kubeConfigPath string
		kvcpImage      string
		found          bool
	)
	if kubeConfigPath, found = os.LookupEnv("KUBECONFIG"); !found {
		kubeConfigPath = filepath.Join(os.Getenv("HOME"), ".kube", "config")
	}

	if kvcpImage, found = os.LookupEnv("KUBE_VIP_CLOUD_PROVIDER_E2E_IMAGE"); !found {
		kvcpImage = "ghcr.io/kube-vip/kube-vip-cloud-provider:main"
	}

	config, err := clientcmd.BuildConfigFromFlags("", kubeConfigPath)
	require.NoError(t, err)
	client, err := kubernetes.NewForConfig(config)
	require.NoError(t, err)

	deployment := &Deployment{
		client:    client,
		kvcpImage: kvcpImage,
	}

	require.NoError(t, deployment.UnmarshalResources())

	// Update deployment's image
	deployment.Deployment.Spec.Template.Spec.Containers[0].Image = kvcpImage
	deployment.Deployment.Spec.Template.Spec.Containers[0].ImagePullPolicy = core_v1.PullIfNotPresent

	return &Framework{
		Client:        client,
		RetryInterval: time.Second,
		RetryTimeout:  60 * time.Second,
		Deployment:    deployment,
		t:             t,
	}
}

// T exposes a GinkgoTInterface which exposes many of the same methods
// as a *testing.T, for use in tests that previously required a *testing.T.
func (f *Framework) T() ginkgo.GinkgoTInterface {
	return f.t
}

type (
	NamespacedTestBody func(string)
)

func (f *Framework) NamespacedTest(namespace string, body NamespacedTestBody, additionalNamespaces ...string) {
	ginkgo.Context("with namespace: "+namespace, func() {
		ginkgo.BeforeEach(func() {
			for _, ns := range append(additionalNamespaces, namespace) {
				f.CreateNamespace(ns)
			}
		})
		ginkgo.AfterEach(func() {
			for _, ns := range append(additionalNamespaces, namespace) {
				f.DeleteNamespace(ns, false)
			}
		})

		body(namespace)
	})
}

// CreateNamespace creates a namespace with the given name in the
// Kubernetes API or fails the test if it encounters an error.
func (f *Framework) CreateNamespace(name string) {
	ns := &core_v1.Namespace{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:   name,
			Labels: map[string]string{"kvcp-e2e-ns": "true"},
		},
	}

	existing, err := f.Client.CoreV1().Namespaces().Get(context.TODO(), name, meta_v1.GetOptions{})
	if err == nil && existing.Status.Phase == core_v1.NamespaceTerminating {
		require.Eventually(f.t, func() bool {
			_, err := f.Client.CoreV1().Namespaces().Get(context.TODO(), name, meta_v1.GetOptions{})
			return api_errors.IsNotFound(err)
		}, 3*time.Minute, time.Second)
	}

	// Now try creating it.
	_, err = f.Client.CoreV1().Namespaces().Create(context.TODO(), ns, meta_v1.CreateOptions{})
	require.NoError(f.t, err)
}

// DeleteNamespace deletes the namespace with the given name in the
// Kubernetes API or fails the test if it encounters an error.
func (f *Framework) DeleteNamespace(name string, waitForDeletion bool) {
	require.NoError(f.t, f.Client.CoreV1().Namespaces().Delete(context.TODO(), name, meta_v1.DeleteOptions{}))

	if waitForDeletion {
		require.Eventually(f.t, func() bool {
			_, err := f.Client.CoreV1().Namespaces().Get(context.TODO(), name, meta_v1.GetOptions{})
			return api_errors.IsNotFound(err)
		}, time.Minute*3, time.Millisecond*50)
	}
}
