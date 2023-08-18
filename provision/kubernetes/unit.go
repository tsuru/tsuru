// Copyright 2022 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package kubernetes

import (
	"context"
	"fmt"

	"github.com/pkg/errors"
	"github.com/tsuru/tsuru/provision"
	policyv1 "k8s.io/api/policy/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func (p *kubernetesProvisioner) KillUnit(ctx context.Context, app provision.App, unitName string, force bool) error {
	clusterClient, err := clusterForPool(ctx, app.GetPool())
	if err != nil {
		return err
	}

	ns, err := clusterClient.AppNamespace(ctx, app)
	if err != nil {
		return err
	}

	pod, err := clusterClient.CoreV1().Pods(ns).Get(ctx, unitName, metav1.GetOptions{})
	if err != nil {
		return errors.Wrap(err, "Unable to find pod")
	}

	appName := app.GetName()
	if pod.Labels["tsuru.io/app-name"] != appName {
		return fmt.Errorf("Unit %q does not belong to app %q", unitName, appName)
	}

	if force {
		err = clusterClient.CoreV1().Pods(ns).Delete(ctx, pod.Name, metav1.DeleteOptions{})
		if err != nil {
			return errors.Wrap(err, "Unable to delete pod")
		}
		return nil
	}

	err = clusterClient.CoreV1().Pods(ns).EvictV1(ctx, &policyv1.Eviction{
		ObjectMeta: metav1.ObjectMeta{
			Name:      pod.Name,
			Namespace: pod.Namespace,
		},
	})
	if err != nil {
		return errors.Wrap(err, "Unable to evict pod")
	}
	return nil
}
