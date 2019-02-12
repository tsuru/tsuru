package util

import (
	"fmt"
	"time"

	errs "github.com/pkg/errors"
	"github.com/rancher/types/apis/management.cattle.io/v3"
	"gopkg.in/yaml.v2"
	"k8s.io/api/core/v1"
	"k8s.io/api/rbac/v1beta1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

const (
	defaultNamespace          = "default"
	cattleNamespace           = "cattle-system"
	clusterAdmin              = "cluster-admin"
	netesDefault              = "netes-default"
	kontainerEngine           = "kontainer-engine"
	oldClusterRoleBindingName = "netes-default-clusterRoleBinding"
	newClusterRoleBindingName = "system-netes-default-clusterRoleBinding"
)

// GenerateServiceAccountToken generate a serviceAccountToken for clusterAdmin given a rest clientset
func GenerateServiceAccountToken(clientset kubernetes.Interface) (string, error) {
	_, err := clientset.CoreV1().Namespaces().Create(&v1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: cattleNamespace,
		},
	})
	if err != nil && !errors.IsAlreadyExists(err) {
		return "", err
	}

	serviceAccount := &v1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name: kontainerEngine,
		},
	}

	_, err = clientset.CoreV1().ServiceAccounts(cattleNamespace).Create(serviceAccount)
	if err != nil && !errors.IsAlreadyExists(err) {
		return "", fmt.Errorf("error creating service account: %v", err)
	}

	adminRole := &v1beta1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{
			Name: clusterAdmin,
		},
		Rules: []v1beta1.PolicyRule{
			{
				APIGroups: []string{"*"},
				Resources: []string{"*"},
				Verbs:     []string{"*"},
			},
			{
				NonResourceURLs: []string{"*"},
				Verbs:           []string{"*"},
			},
		},
	}
	clusterAdminRole, err := clientset.RbacV1beta1().ClusterRoles().Get(clusterAdmin, metav1.GetOptions{})
	if err != nil {
		clusterAdminRole, err = clientset.RbacV1beta1().ClusterRoles().Create(adminRole)
		if err != nil {
			return "", fmt.Errorf("error creating admin role: %v", err)
		}
	}

	clusterRoleBinding := &v1beta1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name: newClusterRoleBindingName,
		},
		Subjects: []v1beta1.Subject{
			{
				Kind:      "ServiceAccount",
				Name:      serviceAccount.Name,
				Namespace: cattleNamespace,
				APIGroup:  v1.GroupName,
			},
		},
		RoleRef: v1beta1.RoleRef{
			Kind:     "ClusterRole",
			Name:     clusterAdminRole.Name,
			APIGroup: v1beta1.GroupName,
		},
	}
	if _, err = clientset.RbacV1beta1().ClusterRoleBindings().Create(clusterRoleBinding); err != nil && !errors.IsAlreadyExists(err) {
		return "", fmt.Errorf("error creating role bindings: %v", err)
	}

	start := time.Millisecond * 250
	for i := 0; i < 5; i++ {
		time.Sleep(start)
		if serviceAccount, err = clientset.CoreV1().ServiceAccounts(cattleNamespace).Get(serviceAccount.Name, metav1.GetOptions{}); err != nil {
			return "", fmt.Errorf("error getting service account: %v", err)
		}

		if len(serviceAccount.Secrets) > 0 {
			secret := serviceAccount.Secrets[0]
			secretObj, err := clientset.CoreV1().Secrets(cattleNamespace).Get(secret.Name, metav1.GetOptions{})
			if err != nil {
				return "", fmt.Errorf("error getting secret: %v", err)
			}
			if token, ok := secretObj.Data["token"]; ok {
				return string(token), nil
			}
		}
		start = start * 2
	}

	return "", errs.New("failed to fetch serviceAccountToken")
}

func DeleteLegacyServiceAccountAndRoleBinding(clientset kubernetes.Interface) error {
	_, err := clientset.CoreV1().ServiceAccounts(defaultNamespace).Get(netesDefault, metav1.GetOptions{})
	if !errors.IsNotFound(err) {
		err = clientset.CoreV1().ServiceAccounts(defaultNamespace).Delete(netesDefault, &metav1.DeleteOptions{})
		if err != nil {
			return err
		}
	}

	_, err = clientset.RbacV1beta1().ClusterRoleBindings().Get(oldClusterRoleBindingName, metav1.GetOptions{})
	if !errors.IsNotFound(err) {
		err = clientset.RbacV1beta1().ClusterRoleBindings().Delete(oldClusterRoleBindingName, &metav1.DeleteOptions{})
		if err != nil {
			return err
		}
	}

	return nil
}

func ConvertToRkeConfig(config string) (v3.RancherKubernetesEngineConfig, error) {
	var rkeConfig v3.RancherKubernetesEngineConfig
	if err := yaml.Unmarshal([]byte(config), &rkeConfig); err != nil {
		return rkeConfig, err
	}
	return rkeConfig, nil
}
