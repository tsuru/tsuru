package kubernetes

import (
	"context"
	"fmt"
	"io"
	"reflect"
	"strings"
	"time"

	"github.com/pkg/errors"
	"github.com/tsuru/tsuru/provision"
	appTypes "github.com/tsuru/tsuru/types/app"
	provTypes "github.com/tsuru/tsuru/types/provision"
	k8sErrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	backendconfigv1 "k8s.io/ingress-gce/pkg/apis/backendconfig/v1"
)

func int64PointerFromInt(v int) *int64 {
	p := int64(v)
	return &p
}

func backendConfigCRDExists(ctx context.Context, client *ClusterClient) (bool, error) {
	return crdExists(ctx, client, backendConfigCRDName)
}

func backendConfigNameForApp(a provision.App, process string) string {
	return provision.AppProcessName(a, process, 0, "")
}

func gcpHCToString(hc *backendconfigv1.HealthCheckConfig) string {
	if hc == nil {
		hc = &backendconfigv1.HealthCheckConfig{}
	}

	hcType := "TCP"
	if hc.Type != nil {
		hcType = *hc.Type
	}

	path := "/"
	if hc.RequestPath != nil {
		path = *hc.RequestPath
	}

	intervalSec := int64(5)
	if hc.CheckIntervalSec != nil {
		intervalSec = *hc.CheckIntervalSec
	}

	timeoutSec := int64(5)
	if hc.TimeoutSec != nil {
		timeoutSec = *hc.TimeoutSec
	}

	success := int64(2)
	if hc.HealthyThreshold != nil {
		success = *hc.HealthyThreshold
	}

	failure := int64(2)
	if hc.UnhealthyThreshold != nil {
		failure = *hc.UnhealthyThreshold
	}

	return fmt.Sprintf("path=%s type=%s interval=%v timeout=%v success=%d failure=%d", path, hcType, time.Duration(intervalSec)*time.Second, time.Duration(timeoutSec)*time.Second, success, failure)
}

func backendConfigFromHC(ctx context.Context, app provision.App, process string, hc provTypes.TsuruYamlHealthcheck) (*backendconfigv1.BackendConfig, error) {
	err := ensureHealthCheckDefaults(&hc)
	if err != nil {
		return nil, err
	}

	if hc.Path == "" {
		hc.Path = "/"
	}
	intervalSec := int64PointerFromInt(hc.IntervalSeconds)
	timeoutSec := int64PointerFromInt(hc.TimeoutSeconds)
	if *timeoutSec >= *intervalSec {
		*intervalSec = *timeoutSec + 1
	}
	protocolType := strings.ToUpper(hc.Scheme)

	labels, err := provision.ServiceLabels(ctx, provision.ServiceLabelsOpts{
		App:     app,
		Process: process,
		ServiceLabelExtendedOpts: provision.ServiceLabelExtendedOpts{
			Prefix:      tsuruLabelPrefix,
			Provisioner: provisionerName,
		},
	})
	if err != nil {
		return nil, errors.WithStack(err)
	}
	labels = labels.WithoutIsolated().WithoutRoutable()

	return &backendconfigv1.BackendConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:   backendConfigNameForApp(app, process),
			Labels: labels.ToLabels(),
		},
		Spec: backendconfigv1.BackendConfigSpec{
			HealthCheck: &backendconfigv1.HealthCheckConfig{
				CheckIntervalSec:   intervalSec,
				TimeoutSec:         timeoutSec,
				Type:               &protocolType,
				RequestPath:        &hc.Path,
				HealthyThreshold:   int64PointerFromInt(1),
				UnhealthyThreshold: int64PointerFromInt(hc.AllowedFailures),
			},
		},
	}, nil
}

type backendConfigArgs struct {
	client  *ClusterClient
	app     provision.App
	process string
	writer  io.Writer
	version appTypes.AppVersion
}

func ensureBackendConfig(ctx context.Context, args backendConfigArgs) (bool, error) {
	crdExists, err := backendConfigCRDExists(ctx, args.client)
	if err != nil {
		return crdExists, err
	}
	if !crdExists {
		return crdExists, nil
	}

	ns, err := args.client.AppNamespace(ctx, args.app)
	if err != nil {
		return crdExists, err
	}

	yamlData, err := args.version.TsuruYamlData()
	if err != nil {
		return crdExists, err
	}

	var hc provTypes.TsuruYamlHealthcheck
	if yamlData.Healthcheck != nil {
		hc = *yamlData.Healthcheck
	}
	backendConfig, err := backendConfigFromHC(ctx, args.app, args.process, hc)
	if err != nil {
		return crdExists, err
	}
	backendConfig.Namespace = ns

	cli, err := BackendConfigClientForConfig(args.client.RestConfig())
	if err != nil {
		return crdExists, err
	}
	existingBackendConfig, err := cli.CloudV1().BackendConfigs(ns).Get(ctx, backendConfig.Name, metav1.GetOptions{})
	if k8sErrors.IsNotFound(err) {
		existingBackendConfig = nil
	} else if err != nil {
		return crdExists, err
	}

	if existingBackendConfig != nil {
		if reflect.DeepEqual(backendConfig.Spec.HealthCheck, existingBackendConfig.Spec.HealthCheck) {
			return crdExists, nil
		}
	}

	newDesc := gcpHCToString(backendConfig.Spec.HealthCheck)
	fmt.Fprint(args.writer, "\n---- GCP Load Balancer health check ----\n")
	if existingBackendConfig != nil {
		existingDesc := gcpHCToString(existingBackendConfig.Spec.HealthCheck)
		fmt.Fprint(args.writer, " ---> Updating LB health check\n")
		fmt.Fprintf(args.writer, " ---> Existing HC: %s\n", existingDesc)
		fmt.Fprintf(args.writer, " --->      New HC: %s\n", newDesc)
		backendConfig.ResourceVersion = existingBackendConfig.ResourceVersion
		_, err = cli.CloudV1().BackendConfigs(ns).Update(ctx, backendConfig, metav1.UpdateOptions{})
	} else {
		fmt.Fprintf(args.writer, " ---> Creating LB health check with %s\n", newDesc)
		_, err = cli.CloudV1().BackendConfigs(ns).Create(ctx, backendConfig, metav1.CreateOptions{})
	}
	if err != nil {
		return crdExists, err
	}

	return crdExists, nil
}

func deleteAllBackendConfig(ctx context.Context, client *ClusterClient, app provision.App) error {
	cli, err := BackendConfigClientForConfig(client.RestConfig())
	if err != nil {
		return err
	}
	ns, err := client.AppNamespace(ctx, app)
	if err != nil {
		return err
	}
	ls, err := provision.ServiceLabels(ctx, provision.ServiceLabelsOpts{
		App: app,
		ServiceLabelExtendedOpts: provision.ServiceLabelExtendedOpts{
			Prefix:      tsuruLabelPrefix,
			Provisioner: provisionerName,
		},
	})
	if err != nil {
		return err
	}
	existingBackendConfigs, err := cli.CloudV1().BackendConfigs(ns).List(ctx, metav1.ListOptions{
		LabelSelector: labels.SelectorFromSet(labels.Set(ls.ToHPASelector())).String(),
	})
	if err != nil {
		if k8sErrors.IsNotFound(err) {
			return nil
		}
		return err
	}

	for _, backendConfig := range existingBackendConfigs.Items {
		err = cli.CloudV1().BackendConfigs(backendConfig.Namespace).Delete(ctx, backendConfig.Name, metav1.DeleteOptions{})
		if err != nil && !k8sErrors.IsNotFound(err) {
			return errors.WithStack(err)
		}
	}
	return nil
}
