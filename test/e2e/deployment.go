//go:build e2e

package e2e

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"time"

	apps_v1 "k8s.io/api/apps/v1"
	core_v1 "k8s.io/api/core/v1"
	rbac_v1 "k8s.io/api/rbac/v1"
	api_errors "k8s.io/apimachinery/pkg/api/errors"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	apimachinery_util_yaml "k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/client-go/kubernetes"
	"k8s.io/utils/ptr"
)

type Deployment struct {
	// Kube-vip-cloud-provider image to use in deployment.
	kvcpImage string
	// client is a client-go Kubernetes client.
	client kubernetes.Interface

	Deployment         *apps_v1.Deployment
	ServiceAccount     *core_v1.ServiceAccount
	ClusterRoleBinding *rbac_v1.ClusterRoleBinding
	ClusterRole        *rbac_v1.ClusterRole
	ConfigMap          *core_v1.ConfigMap
}

// UnmarshalResources unmarshals resources from rendered kube-vip-cloud-provider
// manifest in order.
// rendered deployment manifest.
func (d *Deployment) UnmarshalResources() error {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		return errors.New("could not get path to this source file (test/e2e/deployment.go)")
	}
	renderedDeploymentManifestPath := filepath.Join(filepath.Dir(thisFile), "..", "..", "manifest", "kube-vip-cloud-controller.yaml")
	deploymentFile, err := os.Open(renderedDeploymentManifestPath)
	if err != nil {
		return err
	}
	defer deploymentFile.Close()
	decoder := apimachinery_util_yaml.NewYAMLToJSONDecoder(deploymentFile)

	renderedConfigmapManifestPath := filepath.Join(filepath.Dir(thisFile), "..", "..", "example", "configmap", "0.1.yaml")
	configmapFile, err := os.Open(renderedConfigmapManifestPath)
	if err != nil {
		return err
	}
	defer configmapFile.Close()
	cmDecoder := apimachinery_util_yaml.NewYAMLToJSONDecoder(configmapFile)

	d.ServiceAccount = new(core_v1.ServiceAccount)
	d.ClusterRole = new(rbac_v1.ClusterRole)
	d.ClusterRoleBinding = new(rbac_v1.ClusterRoleBinding)
	d.Deployment = new(apps_v1.Deployment)
	d.ConfigMap = new(core_v1.ConfigMap)

	// Decode content in deployment manifest first.
	// Has to be exact order that the yaml file is constructed.
	objects := []any{
		d.ServiceAccount,
		d.ClusterRole,
		d.ClusterRoleBinding,
		d.Deployment,
	}
	for _, o := range objects {
		if err := decoder.Decode(o); err != nil {
			return err
		}
	}

	// Decode content in configmap manifest.
	if err := cmDecoder.Decode(d.ConfigMap); err != nil {
		return err
	}
	return nil
}

func (d *Deployment) EnsureResources() error {
	if err := d.EnsureClusterRoleBinding(); err != nil {
		return err
	}
	if err := d.EnsureClusterRole(); err != nil {
		return err
	}
	if err := d.EnsureServiceAccount(); err != nil {
		return err
	}
	if err := d.EnsureConfigMap(); err != nil {
		return err
	}
	if err := d.EnsureDeployment(); err != nil {
		return err
	}

	// make sure kube-vip-cloud-provider is running
	if err := d.WaitForPodRunning(); err != nil {
		return err
	}

	return nil
}

func (d *Deployment) EnsureServiceAccount() error {
	// Common case of updating object if exists, create otherwise.
	newObj := d.ServiceAccount
	curObj, err := d.client.CoreV1().ServiceAccounts(newObj.Namespace).Get(context.TODO(), newObj.Name, meta_v1.GetOptions{})
	if err != nil {
		if api_errors.IsNotFound(err) {
			_, err = d.client.CoreV1().ServiceAccounts(newObj.Namespace).Create(context.TODO(), newObj, meta_v1.CreateOptions{})
			return err
		}
		return err
	}

	newObj.SetResourceVersion(curObj.GetResourceVersion())
	_, err = d.client.CoreV1().ServiceAccounts(newObj.Namespace).Update(context.TODO(), newObj, meta_v1.UpdateOptions{})

	return err
}

func (d *Deployment) EnsureClusterRoleBinding() error {
	// Common case of updating object if exists, create otherwise.
	newObj := d.ClusterRoleBinding
	curObj, err := d.client.RbacV1().ClusterRoleBindings().Get(context.TODO(), newObj.Name, meta_v1.GetOptions{})
	if err != nil {
		if api_errors.IsNotFound(err) {
			_, err = d.client.RbacV1().ClusterRoleBindings().Create(context.TODO(), newObj, meta_v1.CreateOptions{})
			return err
		}
	}

	newObj.SetResourceVersion(curObj.GetResourceVersion())
	_, err = d.client.RbacV1().ClusterRoleBindings().Update(context.TODO(), newObj, meta_v1.UpdateOptions{})

	return err
}

func (d *Deployment) EnsureClusterRole() error {
	// Common case of updating object if exists, create otherwise.
	newObj := d.ClusterRole
	curObj, err := d.client.RbacV1().ClusterRoles().Get(context.TODO(), newObj.Name, meta_v1.GetOptions{})
	if err != nil {
		if api_errors.IsNotFound(err) {
			_, err = d.client.RbacV1().ClusterRoles().Create(context.TODO(), newObj, meta_v1.CreateOptions{})
			return err
		}
	}

	newObj.SetResourceVersion(curObj.GetResourceVersion())
	_, err = d.client.RbacV1().ClusterRoles().Update(context.TODO(), newObj, meta_v1.UpdateOptions{})

	return err
}

func (d *Deployment) EnsureConfigMap() error {
	// Common case of updating object if exists, create otherwise.
	newObj := d.ConfigMap
	curObj, err := d.client.CoreV1().ConfigMaps(newObj.Namespace).Get(context.TODO(), newObj.Name, meta_v1.GetOptions{})
	if err != nil {
		if api_errors.IsNotFound(err) {
			_, err = d.client.CoreV1().ConfigMaps(newObj.Namespace).Create(context.TODO(), newObj, meta_v1.CreateOptions{})
			return err
		}
	}

	newObj.SetResourceVersion(curObj.GetResourceVersion())
	_, err = d.client.CoreV1().ConfigMaps(newObj.Namespace).Update(context.TODO(), newObj, meta_v1.UpdateOptions{})

	return err
}

func (d *Deployment) EnsureDeployment() error {
	// Common case of updating object if exists, create otherwise.
	newObj := d.Deployment
	curObj, err := d.client.AppsV1().Deployments(newObj.Namespace).Get(context.TODO(), newObj.Name, meta_v1.GetOptions{})
	if err != nil {
		if api_errors.IsNotFound(err) {
			_, err = d.client.AppsV1().Deployments(newObj.Namespace).Create(context.TODO(), newObj, meta_v1.CreateOptions{})
			return err
		}
	}

	newObj.SetResourceVersion(curObj.GetResourceVersion())
	_, err = d.client.AppsV1().Deployments(newObj.Namespace).Update(context.TODO(), newObj, meta_v1.UpdateOptions{})

	return err
}

func (d *Deployment) WaitForPodRunning() error {
	updatedPods := func(ctx context.Context) (bool, error) {
		dp, err := d.client.AppsV1().Deployments(d.Deployment.Namespace).Get(context.TODO(), d.Deployment.Name, meta_v1.GetOptions{})
		if err != nil {
			return false, err
		}
		runningPodNum := d.PodWithStatusRunning(ctx)

		return int(dp.Status.ReadyReplicas) == int(*dp.Spec.Replicas) &&
			int(dp.Status.UnavailableReplicas) == 0 &&
			int(dp.Status.AvailableReplicas) == int(*dp.Spec.Replicas) &&
			runningPodNum == int(*dp.Spec.Replicas), nil
	}
	return wait.PollUntilContextTimeout(context.Background(), time.Millisecond*50, time.Minute*3, true, updatedPods)
}

func (d *Deployment) PodWithStatusRunning(ctx context.Context) int {
	pods := new(core_v1.PodList)
	listOptions := meta_v1.ListOptions{
		LabelSelector: "component=kube-vip-cloud-provider",
	}
	pods, err := d.client.CoreV1().Pods("").List(ctx, listOptions)
	if err != nil {
		return 0
	}
	c := 0
	for _, pod := range pods.Items {
		if pod.Status.Phase == core_v1.PodRunning {
			c++
		}
	}
	return c
}

func (d *Deployment) DeleteResources() error {
	if err := d.DeleteServiceAccount(); err != nil {
		return err
	}
	if err := d.DeleteClusterRoleBinding(); err != nil {
		return err
	}
	if err := d.DeleteClusterRole(); err != nil {
		return err
	}
	if err := d.DeleteConfigMap(); err != nil {
		return err
	}
	if err := d.DeleteDeployment(); err != nil {
		return err
	}
	return nil
}

func (d *Deployment) DeleteServiceAccount() error {
	// Common case of updating object if exists, create otherwise.
	err := d.client.CoreV1().ServiceAccounts(d.ServiceAccount.Namespace).Delete(context.TODO(), d.ServiceAccount.Name,
		*&meta_v1.DeleteOptions{PropagationPolicy: ptr.To(meta_v1.DeletePropagationBackground)})
	if api_errors.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("error deleting ServiceAccount %s/%s: %v", d.ServiceAccount.Namespace, d.ServiceAccount.Name, err)
	}

	// Wait to ensure it's fully deleted.
	if err = wait.PollUntilContextTimeout(context.Background(), 100*time.Millisecond, time.Minute, true, func(ctx context.Context) (bool, error) {
		_, err = d.client.CoreV1().ServiceAccounts(d.ServiceAccount.Namespace).Get(context.TODO(), d.ServiceAccount.Name, meta_v1.GetOptions{})
		if api_errors.IsNotFound(err) {
			return true, nil
		}
		return false, nil
	}); err != nil {
		return fmt.Errorf("error waiting for deletion of ServiceAccount %s/%s: %v", d.ServiceAccount.Namespace, d.ServiceAccount.Name, err)
	}

	// Clear out resource version to ensure object can be used again.
	d.ServiceAccount.SetResourceVersion("")

	return err
}

func (d *Deployment) DeleteClusterRoleBinding() error {
	// Common case of updating object if exists, create otherwise.
	err := d.client.RbacV1().ClusterRoleBindings().Delete(context.TODO(), d.ClusterRoleBinding.Name,
		*&meta_v1.DeleteOptions{PropagationPolicy: ptr.To(meta_v1.DeletePropagationBackground)})
	if api_errors.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("error deleting ClusterRoleBinding %s/%s: %v", d.ClusterRoleBinding.Namespace, d.ClusterRoleBinding.Name, err)
	}
	// Wait to ensure it's fully deleted.
	if err = wait.PollUntilContextTimeout(context.Background(), 100*time.Millisecond, time.Minute, true, func(ctx context.Context) (bool, error) {
		_, err = d.client.RbacV1().ClusterRoleBindings().Get(context.TODO(), d.ClusterRoleBinding.Name, meta_v1.GetOptions{})
		if api_errors.IsNotFound(err) {
			return true, nil
		}
		return false, nil
	}); err != nil {
		return fmt.Errorf("error waiting for deletion of ClusterRoleBinding %s/%s: %v", d.ClusterRoleBinding.Namespace, d.ClusterRoleBinding.Name, err)
	} // Clear out resource version to ensure object can be used again.
	d.ClusterRoleBinding.SetResourceVersion("")

	return err
}

func (d *Deployment) DeleteClusterRole() error {
	// Common case of updating object if exists, create otherwise.
	err := d.client.RbacV1().ClusterRoles().Delete(context.TODO(), d.ClusterRole.Name,
		*&meta_v1.DeleteOptions{PropagationPolicy: ptr.To(meta_v1.DeletePropagationBackground)})
	if api_errors.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("error deleting ClusterRole %s/%s: %v", d.ClusterRole.Namespace, d.ClusterRole.Name, err)
	}

	// Wait to ensure it's fully deleted.
	if err = wait.PollUntilContextTimeout(context.Background(), 100*time.Millisecond, time.Minute, true, func(ctx context.Context) (bool, error) {
		_, err = d.client.RbacV1().ClusterRoles().Get(context.TODO(), d.ClusterRole.Name, meta_v1.GetOptions{})
		if api_errors.IsNotFound(err) {
			return true, nil
		}
		return false, nil
	}); err != nil {
		return fmt.Errorf("error waiting for deletion of ClusterRole %s/%s: %v", d.ClusterRole.Namespace, d.ClusterRole.Name, err)
	}

	// Clear out resource version to ensure object can be used again.
	d.ClusterRole.SetResourceVersion("")

	return err
}

func (d *Deployment) DeleteConfigMap() error {
	// Common case of updating object if exists, create otherwise.
	err := d.client.CoreV1().ConfigMaps(d.ConfigMap.Namespace).Delete(context.TODO(), d.ConfigMap.Name,
		*&meta_v1.DeleteOptions{PropagationPolicy: ptr.To(meta_v1.DeletePropagationBackground)})
	if api_errors.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("error deleting ConfigMap %s/%s: %v", d.ConfigMap.Namespace, d.ConfigMap.Name, err)
	}

	// Wait to ensure it's fully deleted.
	if err = wait.PollUntilContextTimeout(context.Background(), 100*time.Millisecond, time.Minute, true, func(ctx context.Context) (bool, error) {
		_, err = d.client.CoreV1().ConfigMaps(d.ConfigMap.Namespace).Get(context.TODO(), d.ConfigMap.Name, meta_v1.GetOptions{})
		if api_errors.IsNotFound(err) {
			return true, nil
		}
		return false, nil
	}); err != nil {
		return fmt.Errorf("error waiting for deletion of ConfigMap %s/%s: %v", d.ConfigMap.Namespace, d.ConfigMap.Name, err)
	}

	// Clear out resource version to ensure object can be used again.
	d.ConfigMap.SetResourceVersion("")

	return err
}

func (d *Deployment) DeleteDeployment() error {
	// Common case of updating object if exists, create otherwise.
	err := d.client.AppsV1().Deployments(d.Deployment.Namespace).Delete(context.TODO(), d.Deployment.Name,
		*&meta_v1.DeleteOptions{PropagationPolicy: ptr.To(meta_v1.DeletePropagationBackground)})
	if api_errors.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("error deleting Deployment %s/%s: %v", d.Deployment.Namespace, d.Deployment.Name, err)
	}

	// Wait to ensure it's fully deleted.
	if err = wait.PollUntilContextTimeout(context.Background(), 100*time.Millisecond, time.Minute, true, func(ctx context.Context) (bool, error) {
		_, err = d.client.AppsV1().Deployments(d.Deployment.Namespace).Get(context.TODO(), d.Deployment.Name, meta_v1.GetOptions{})
		if api_errors.IsNotFound(err) {
			return true, nil
		}
		return false, nil
	}); err != nil {
		return fmt.Errorf("error waiting for deletion of Deployment %s/%s: %v", d.Deployment.Namespace, d.Deployment.Name, err)
	}

	// Clear out resource version to ensure object can be used again.
	d.Deployment.SetResourceVersion("")

	return err
}
