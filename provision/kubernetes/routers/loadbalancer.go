// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package routers

import (
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/tsuru/kubernetes-router/router"
	tsuruv1 "github.com/tsuru/tsuru/provision/kubernetes/pkg/apis/tsuru/v1"
	v1 "k8s.io/api/core/v1"
	k8sErrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	appTypes "github.com/tsuru/tsuru/types/app"
)

const (
	// defaultLBPort is the default exposed port to the LB
	defaultLBPort = 80

	// exposeAllPortsOpt is the flag used to expose all ports in the LB
	exposeAllPortsOpt = "expose-all-ports"
)

var (
	// ErrLoadBalancerNotReady is returned when a given LB has no IP
	ErrLoadBalancerNotReady = errors.New("load balancer is not ready")
)

type KubeRouter interface {
	EnsureRouter(app appTypes.App, opts map[string]string) error
}

// LBService manages LoadBalancer services
type LBService struct {
	*BaseService

	// // OptsAsLabels maps router additional options to labels to be set on the service
	// OptsAsLabels map[string]string

	// // OptsAsLabelsDocs maps router additional options to user friendly help text
	// OptsAsLabelsDocs map[string]string

	// // PoolLabels maps router additional options for a given pool to be set on the service
	// PoolLabels map[string]map[string]string
}

// Remove removes the LoadBalancer service
// func (s *LBService) Remove(id router.InstanceID) error {
//	client, err := s.getClient()
//	if err != nil {
//		return err
//	}
//	service, err := s.getLBService(id)
//	if err != nil {
//		if k8sErrors.IsNotFound(err) {
//			return nil
//		}
//		return err
//	}
//	if dstApp, swapped := s.BaseService.isSwapped(service.ObjectMeta); swapped {
//		return ErrAppSwapped{App: id.AppName, DstApp: dstApp}
//	}
//	ns, err := s.getAppNamespace(id.AppName)
//	if err != nil {
//		return err
//	}
//	err = client.CoreV1().Services(ns).Delete(service.Name, &metav1.DeleteOptions{})
//	if k8sErrors.IsNotFound(err) {
//		return nil
//	}
//	return err
// }

// // Swap swaps the two LB services selectors
// func (s *LBService) Swap(srcID, dstID router.InstanceID) error {
//	srcServ, err := s.getLBService(srcID)
//	if err != nil {
//		return err
//	}
//	if !isReady(srcServ) {
//		return ErrLoadBalancerNotReady
//	}
//	dstServ, err := s.getLBService(dstID)
//	if err != nil {
//		return err
//	}
//	if !isReady(dstServ) {
//		return ErrLoadBalancerNotReady
//	}
//	s.swap(srcServ, dstServ)
//	client, err := s.getClient()
//	if err != nil {
//		return err
//	}
//	ns, err := s.getAppNamespace(srcID.AppName)
//	if err != nil {
//		return err
//	}
//	ns2, err := s.getAppNamespace(dstID.AppName)
//	if err != nil {
//		return err
//	}
//	if ns != ns2 {
//		return fmt.Errorf("unable to swap apps with different namespaces: %v != %v", ns, ns2)
//	}
//	_, err = client.CoreV1().Services(ns).Update(srcServ)
//	if err != nil {
//		return err
//	}
//	_, err = client.CoreV1().Services(ns).Update(dstServ)
//	if err != nil {
//		s.swap(srcServ, dstServ)
//		_, errRollback := client.CoreV1().Services(ns).Update(srcServ)
//		if errRollback != nil {
//			return fmt.Errorf("failed to rollback swap %v: %v", err, errRollback)
//		}
//	}
//	return err
// }

// // Get returns the LoadBalancer IP
// func (s *LBService) GetAddresses(id router.InstanceID) ([]string, error) {
//	service, err := s.getLBService(id)
//	if err != nil {
//		return nil, err
//	}
//	var addr string
//	lbs := service.Status.LoadBalancer.Ingress
//	if len(lbs) != 0 {
//		addr = lbs[0].IP
//		ports := service.Spec.Ports
//		if len(ports) != 0 {
//			addr = fmt.Sprintf("%s:%d", addr, ports[0].Port)
//		}
//		if lbs[0].Hostname != "" {
//			addr = lbs[0].Hostname
//		}
//	}
//	return []string{addr}, nil
// }

// SupportedOptions returns all the supported options
// func (s *LBService) SupportedOptions() map[string]string {
//	opts := map[string]string{
//		router.ExposedPort: "",
//		exposeAllPortsOpt:  "Expose all ports used by application in the Load Balancer. Defaults to false.",
//	}
//	for k, v := range s.OptsAsLabels {
//		opts[k] = v
//		if s.OptsAsLabelsDocs[k] != "" {
//			opts[k] = s.OptsAsLabelsDocs[k]
//		}
//	}
//	return opts
// }

func (s *LBService) getLBService(app appTypes.App) (*v1.Service, error) {
	ns, err := s.getAppNamespace(app.GetName())
	if err != nil {
		return nil, err
	}
	return s.Client.CoreV1().Services(ns).Get(s.serviceName(app), metav1.GetOptions{})
}

// func (s *LBService) swap(srcServ, dstServ *v1.Service) {
//	srcServ.Spec.Selector, dstServ.Spec.Selector = dstServ.Spec.Selector, srcServ.Spec.Selector
//	s.BaseService.swap(&srcServ.ObjectMeta, &dstServ.ObjectMeta)
// }

func (s *LBService) serviceName(app appTypes.App) string {
	return s.hashedResourceName(fmt.Sprintf("%s-router-lb", app.GetName()), 63)
}

func isReady(service *v1.Service) bool {
	if len(service.Status.LoadBalancer.Ingress) == 0 {
		return false
	}
	return service.Status.LoadBalancer.Ingress[0].IP != ""
}

func (s *LBService) EnsureRouter(app appTypes.App, opts map[string]string) error {
	lbService, err := s.getLBService(app)
	if err != nil {
		if !k8sErrors.IsNotFound(err) {
			return err
		}
		ns, err := s.getAppNamespace(app.GetName())
		if err != nil {
			return err
		}
		lbService = &v1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      s.serviceName(app),
				Namespace: ns,
			},
			Spec: v1.ServiceSpec{
				Type: v1.ServiceTypeLoadBalancer,
			},
		}
	}
	if _, isSwapped := s.isSwapped(lbService.ObjectMeta); isSwapped {
		return nil
	}

	// if opts == nil {
	//	var annotationOpts router.Opts
	//	annotationOpts, err = router.OptsFromAnnotations(&lbService.ObjectMeta)
	//	if err != nil {
	//		return err
	//	}
	//	opts = &annotationOpts
	// }

	webService, err := s.getWebService(app.GetName(), extraData, lbService.Labels)
	if err != nil {
		if _, isNotFound := err.(ErrNoService); isUpdate || !isNotFound {
			return err
		}
	}
	if webService != nil {
		lbService.Spec.Selector = webService.Spec.Selector
	}

	err = s.fillLabelsAndAnnotations(lbService, id.AppName, webService, *opts, extraData)
	if err != nil {
		return err
	}

	ports, err := s.portsForService(lbService, app, *opts, webService)
	if err != nil {
		return err
	}
	lbService.Spec.Ports = ports

	client, err := s.getClient()
	if err != nil {
		return err
	}
	_, err = client.CoreV1().Services(lbService.Namespace).Update(lbService)
	if k8sErrors.IsNotFound(err) {
		_, err = client.CoreV1().Services(lbService.Namespace).Create(lbService)
	}
	return err
}

func (s *LBService) fillLabelsAndAnnotations(svc *v1.Service, appName string, webService *v1.Service, opts router.Opts, extraData router.RoutesRequestExtraData) error {
	optsLabels := make(map[string]string)
	registeredOpts := s.SupportedOptions()

	optsAnnotations, err := opts.ToAnnotations()
	if err != nil {
		return err
	}
	annotations := mergeMaps(s.Annotations, optsAnnotations)

	for optName, optValue := range opts.AdditionalOpts {
		if labelName, ok := s.OptsAsLabels[optName]; ok {
			optsLabels[labelName] = optValue
			continue
		}
		if _, ok := registeredOpts[optName]; ok {
			continue
		}

		if strings.HasSuffix(optName, "-") {
			delete(annotations, strings.TrimSuffix(optName, "-"))
		} else {
			annotations[optName] = optValue
		}
	}

	labels := []map[string]string{
		svc.Labels,
		s.PoolLabels[opts.Pool],
		optsLabels,
		s.Labels,
		{
			appLabel:             appName,
			managedServiceLabel:  "true",
			externalServiceLabel: "true",
			appPoolLabel:         opts.Pool,
		},
	}

	if webService != nil {
		labels = append(labels, webService.Labels)
		annotations = mergeMaps(annotations, webService.Annotations)
	}

	if extraData.Namespace != "" && extraData.Service != "" {
		labels = append(labels, map[string]string{
			appBaseServiceNamespaceLabel: extraData.Namespace,
			appBaseServiceNameLabel:      extraData.Service,
		})
	}

	svc.Labels = mergeMaps(labels...)
	svc.Annotations = annotations
	return nil
}

func (s *LBService) portsForService(svc *v1.Service, app *tsuruv1.App, opts router.Opts, baseSvc *v1.Service) ([]v1.ServicePort, error) {
	additionalPort, _ := strconv.Atoi(opts.ExposedPort)
	if additionalPort == 0 {
		additionalPort = defaultLBPort
	}

	existingPorts := map[int32]*v1.ServicePort{}
	for i, port := range svc.Spec.Ports {
		existingPorts[port.Port] = &svc.Spec.Ports[i]
	}

	wantedPorts := map[int32]*v1.ServicePort{
		int32(additionalPort): {
			Name:       fmt.Sprintf("port-%d", additionalPort),
			Protocol:   v1.ProtocolTCP,
			Port:       int32(additionalPort),
			TargetPort: intstr.FromInt(getAppServicePort(app)),
		},
	}

	allPorts, _ := strconv.ParseBool(opts.AdditionalOpts[exposeAllPortsOpt])
	if allPorts && baseSvc != nil {
		basePorts := baseSvc.Spec.Ports
		for i := range basePorts {
			if basePorts[i].Port == int32(additionalPort) {
				// Skipping ports conflicting with additional port
				continue
			}
			basePorts[i].NodePort = 0
			wantedPorts[basePorts[i].Port] = &basePorts[i]
		}
	}

	var ports []v1.ServicePort
	for _, wantedPort := range wantedPorts {
		existingPort, ok := existingPorts[wantedPort.Port]
		if ok {
			wantedPort.NodePort = existingPort.NodePort
		}
		ports = append(ports, *wantedPort)
	}
	sort.Slice(ports, func(i, j int) bool {
		return ports[i].Port < ports[j].Port
	})

	return ports, nil
}

func mergeMaps(entries ...map[string]string) map[string]string {
	result := make(map[string]string)
	for _, entry := range entries {
		for k, v := range entry {
			if _, isSet := result[k]; !isSet {
				result[k] = v
			}
		}
	}
	return result
}
