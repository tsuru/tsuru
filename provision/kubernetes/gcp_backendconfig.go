package kubernetes

import (
	"context"
	"strings"

	"github.com/tsuru/tsuru/provision"
	provTypes "github.com/tsuru/tsuru/types/provision"
	k8sErrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	backendconfigv1 "k8s.io/ingress-gce/pkg/apis/backendconfig/v1"
)

func int64PointerFromInt(v int) *int64 {
	p := int64(v)
	return &p
}

func backendConfigCRDExists(ctx context.Context, client *ClusterClient) (bool, error) {
	extClient, err := ExtensionsClientForConfig(client.restConfig)
	if err != nil {
		return false, err
	}
	_, err = extClient.ApiextensionsV1beta1().CustomResourceDefinitions().Get(ctx, backendConfigCRDName, metav1.GetOptions{})
	if k8sErrors.IsNotFound(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

func backendConfigNameForApp(a provision.App, process string) string {
	return provision.AppProcessName(a, process, 0, "")
}

func ensureBackendConfig(ctx context.Context, client *ClusterClient, a provision.App, processName string, hc *provTypes.TsuruYamlHealthcheck) error {
	exists, err := backendConfigCRDExists(ctx, client)
	if err != nil {
		return err
	}
	if !exists {
		return nil
	}

	backendConfigName := backendConfigNameForApp(a, processName)
	cli, err := BackendConfigClientForConfig(client.RestConfig())
	if err != nil {
		return err
	}
	ns, err := client.AppNamespace(ctx, a)
	if err != nil {
		return err
	}

	intervalSec := int64PointerFromInt(hc.IntervalSeconds)
	timeoutSec := int64PointerFromInt(hc.TimeoutSeconds)
	if *timeoutSec >= *intervalSec {
		*intervalSec = *timeoutSec + 1
	}
	protocolType := strings.ToUpper(hc.Scheme)

	backendConfig := &backendconfigv1.BackendConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:      backendConfigName,
			Namespace: ns,
		},
		Spec: backendconfigv1.BackendConfigSpec{
			HealthCheck: &backendconfigv1.HealthCheckConfig{
				CheckIntervalSec: intervalSec,
				TimeoutSec:       timeoutSec,
				Type:             &protocolType,
				RequestPath:      &hc.Path,
			},
		},
	}

	existingBackendConfig, err := cli.CloudV1().BackendConfigs(ns).Get(ctx, backendConfigName, metav1.GetOptions{})
	if k8sErrors.IsNotFound(err) {
		existingBackendConfig = nil
	} else if err != nil {
		return err
	}

	if existingBackendConfig != nil {
		backendConfig.ResourceVersion = existingBackendConfig.ResourceVersion
		_, err = cli.CloudV1().BackendConfigs(ns).Update(ctx, backendConfig, metav1.UpdateOptions{})
	} else {
		_, err = cli.CloudV1().BackendConfigs(ns).Create(ctx, backendConfig, metav1.CreateOptions{})
	}
	if err != nil {
		return err
	}

	return nil
}
