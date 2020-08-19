// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package routers

import (
	"crypto/sha256"
	"fmt"
	"log"

	"github.com/tsuru/kubernetes-router/router"
	tsuruv1 "github.com/tsuru/tsuru/provision/kubernetes/pkg/apis/tsuru/v1"
	"github.com/tsuru/tsuru/types/provision"
	apiv1 "k8s.io/api/core/v1"
	k8sErrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"
	"k8s.io/client-go/kubernetes"
)

const (
	// managedServiceLabel is added to every service created by the router
	managedServiceLabel = "tsuru.io/router-lb"
	// externalServiceLabel should be added to every service with tsuru app
	// labels that are NOT created or managed by tsuru itself.
	externalServiceLabel = "tsuru.io/external-controller"
	headlessServiceLabel = "tsuru.io/is-headless-service"

	appBaseServiceNamespaceLabel = "router.tsuru.io/base-service-namespace"
	appBaseServiceNameLabel      = "router.tsuru.io/base-service-name"

	defaultServicePort = 8888
	appLabel           = "tsuru.io/app-name"
	domainLabel        = "tsuru.io/domain-name"
	processLabel       = "tsuru.io/app-process"
	swapLabel          = "tsuru.io/swapped-with"
	appPoolLabel       = "tsuru.io/app-pool"
	webProcessName     = "web"
)

// ErrNoService indicates that the app has no service running
type ErrNoService struct{ App, Process string }

func (e ErrNoService) Error() string {
	str := fmt.Sprintf("no service found for app %q", e.App)
	if e.Process != "" {
		str += fmt.Sprintf(" and process %q", e.Process)
	}
	return str
}

// ErrAppSwapped indicates when a operation cant be performed
// because the app is swapped
type ErrAppSwapped struct{ App, DstApp string }

func (e ErrAppSwapped) Error() string {
	return fmt.Sprintf("app %q currently swapped with %q", e.App, e.DstApp)
}

// BaseService has the base functionality needed by router.Service implementations
// targeting kubernetes
type BaseService struct {
	Namespace string
	Client    kubernetes.Interface
	// Labels      map[string]string
	// Annotations map[string]string
}

// SupportedOptions returns the options supported by all services
func (k *BaseService) SupportedOptions() map[string]string {
	return nil
}

// Healthcheck uses the kubernetes client to check the connectivity
func (k *BaseService) Healthcheck() error {
	_, err := k.Client.CoreV1().Services(k.Namespace).List(metav1.ListOptions{})
	return err
}

func (k *BaseService) getWebService(appName string, extraData router.RoutesRequestExtraData, currentLabels map[string]string) (*apiv1.Service, error) {
	if currentLabels != nil && extraData.Namespace == "" && extraData.Service == "" {
		extraData.Namespace = currentLabels[appBaseServiceNamespaceLabel]
		extraData.Service = currentLabels[appBaseServiceNameLabel]
	}

	if extraData.Namespace != "" && extraData.Service != "" {
		var svc *apiv1.Service
		svc, err := k.Client.CoreV1().Services(extraData.Namespace).Get(extraData.Service, metav1.GetOptions{})
		if err != nil {
			if k8sErrors.IsNotFound(err) {
				return nil, ErrNoService{App: appName}
			}
			return nil, err
		}
		return svc, nil
	}

	namespace, err := k.getAppNamespace(appName)
	if err != nil {
		return nil, err
	}
	sel, err := makeWebSvcSelector(appName)
	if err != nil {
		return nil, err
	}
	list, err := k.Client.CoreV1().Services(namespace).List(metav1.ListOptions{
		LabelSelector: sel.String(),
	})
	if err != nil {
		return nil, err
	}
	if len(list.Items) == 0 {
		return nil, ErrNoService{App: appName}
	}
	if len(list.Items) == 1 {
		return &list.Items[0], nil
	}
	var service *apiv1.Service
	var webSvcsCounter int
	for i := range list.Items {
		if list.Items[i].Labels[processLabel] == webProcessName {
			webSvcsCounter++
			service = &list.Items[i]
		}
	}
	if webSvcsCounter > 1 {
		log.Printf("WARNING: multiple (%d) services matching app %q and process %q", webSvcsCounter, appName, webProcessName)
		return nil, ErrNoService{App: appName, Process: webProcessName}
	}
	if service != nil {
		return service, nil
	}
	return nil, ErrNoService{App: appName, Process: webProcessName}
}

func (k *BaseService) swap(src, dst *metav1.ObjectMeta) {
	if src.Labels[swapLabel] == dst.Labels[appLabel] {
		src.Labels[swapLabel] = ""
		dst.Labels[swapLabel] = ""
	} else {
		src.Labels[swapLabel] = dst.Labels[appLabel]
		dst.Labels[swapLabel] = src.Labels[appLabel]
	}
}

func (k *BaseService) isSwapped(obj metav1.ObjectMeta) (string, bool) {
	target := obj.Labels[swapLabel]
	return target, target != ""
}

func (k *BaseService) getAppNamespace(appName string) (string, error) {
	return k.Namespace, nil
}

func getAppServicePort(app *tsuruv1.App) int {
	servicePort := defaultServicePort
	if app == nil || app.Spec.Configs == nil {
		return servicePort
	}

	var process *provision.TsuruYamlKubernetesProcessConfig
	for _, group := range app.Spec.Configs.Groups {
		for procName, proc := range group {
			if procName == webProcessName {
				process = &proc
				break
			}
		}
	}
	if process == nil {
		for _, group := range app.Spec.Configs.Groups {
			for _, proc := range group {
				process = &proc
				break
			}
		}
	}
	if process != nil && len(process.Ports) > 0 {
		servicePort = process.Ports[0].TargetPort
		if servicePort == 0 {
			servicePort = process.Ports[0].Port
		}
	}
	return servicePort
}

func makeWebSvcSelector(appName string) (labels.Selector, error) {
	reqs := []struct {
		key string
		op  selection.Operator
		val string
	}{
		{appLabel, selection.Equals, appName},
		{managedServiceLabel, selection.NotEquals, "true"},
		{externalServiceLabel, selection.NotEquals, "true"},
		{headlessServiceLabel, selection.NotEquals, "true"},
	}

	sel := labels.NewSelector()
	for _, reqInfo := range reqs {
		req, err := labels.NewRequirement(reqInfo.key, reqInfo.op, []string{reqInfo.val})
		if err != nil {
			return nil, err
		}
		sel = sel.Add(*req)
	}
	return sel, nil
}

func (s *BaseService) hashedResourceName(name string, limit int) string {
	if len(name) <= limit {
		return name
	}

	h := sha256.New()
	h.Write([]byte(name))
	hash := fmt.Sprintf("%x", h.Sum(nil))
	return fmt.Sprintf("%s-%s", name[:limit-17], hash[:16])
}
