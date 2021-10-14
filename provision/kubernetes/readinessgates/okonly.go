package readinessgates

import (
	"context"
	"strconv"
	"sync"
	"time"

	"github.com/pkg/errors"
	"github.com/tsuru/tsuru/log"
	corev1 "k8s.io/api/core/v1"
	k8sErrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	k8sNet "k8s.io/apimachinery/pkg/util/net"
	corev1informers "k8s.io/client-go/informers/core/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/util/workqueue"
)

const (
	OKOnlyReadinessGateName = corev1.PodConditionType("app.tsuru.io/probe-200-only")
	okOnlyWorkers           = 10
)

type OKOnly struct {
	queue    workqueue.RateLimitingInterface
	wg       sync.WaitGroup
	informer corev1informers.PodInformer
	config   *rest.Config
	client   kubernetes.Interface
}

func NewOKOnlyReadinessGate(informer corev1informers.PodInformer, client kubernetes.Interface, restConfig *rest.Config) *OKOnly {
	okonly := &OKOnly{
		queue: workqueue.NewNamedRateLimitingQueue(
			workqueue.DefaultControllerRateLimiter(),
			"tsuru_workqueue_okonly",
		),
		informer: informer,
		config:   restConfig,
		client:   client,
	}
	okonly.runWorkers()
	return okonly
}

func (o *OKOnly) OnPodEvent(pod *corev1.Pod) {
	if pod.Spec.ReadinessGates == nil {
		return
	}
	for _, cond := range pod.Status.Conditions {
		if cond.Type == OKOnlyReadinessGateName {
			if cond.Status == corev1.ConditionTrue {
				return
			}
		}
	}
	for _, readinessGate := range pod.Spec.ReadinessGates {
		if readinessGate.ConditionType == OKOnlyReadinessGateName {
			o.queue.Add(types.NamespacedName{
				Name:      pod.Name,
				Namespace: pod.Namespace,
			})
			return
		}
	}
}

func (o *OKOnly) process(key interface{}) (err error) {
	podKey, ok := key.(types.NamespacedName)
	if !ok {
		log.Errorf("[okonly] invalid pod key %v - %T", key, key)
		return nil
	}

	ctx := context.Background()

	pod, err := o.informer.Lister().Pods(podKey.Namespace).Get(podKey.Name)
	if err != nil {
		if k8sErrors.IsNotFound(err) {
			return nil
		}
		return err
	}

	newCondition := &corev1.PodCondition{
		Type:               OKOnlyReadinessGateName,
		Status:             corev1.ConditionFalse,
		LastProbeTime:      metav1.Now(),
		LastTransitionTime: metav1.Now(),
	}

	err = checkPod(ctx, o.config, pod)
	if err == nil {
		newCondition.Status = corev1.ConditionTrue
	} else {
		newCondition.Status = corev1.ConditionFalse
		newCondition.Reason = "Failed"
		newCondition.Message = err.Error()
	}

	updatePodCondition(&pod.Status, newCondition)
	_, err = o.client.CoreV1().Pods(podKey.Namespace).UpdateStatus(ctx, pod, metav1.UpdateOptions{})
	if err != nil {
		return err
	}

	return nil
}

func (o *OKOnly) runWorkers() {
	for i := 0; i < okOnlyWorkers; i++ {
		o.wg.Add(1)
		go o.runConsumer()
	}
}

func (o *OKOnly) runConsumer() {
	defer o.wg.Done()
	for {
		shutdown := o.consumer()
		if shutdown {
			return
		}
	}
}

func (o *OKOnly) consumer() bool {
	key, shutdown := o.queue.Get()
	if shutdown {
		return true
	}
	defer o.queue.Done(key)
	err := o.process(key)
	if err == nil {
		o.queue.Forget(key)
		return false
	}

	log.Errorf("[okonly] error processing pod %v: %s", key, err)
	o.queue.AddRateLimited(key)
	return false
}

func (o *OKOnly) Shutdown(ctx context.Context) error {
	o.queue.ShutDown()
	done := make(chan struct{})
	go func() {
		o.wg.Wait()
		close(done)
	}()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-done:
	}
	return nil
}

func getPodCondition(status *corev1.PodStatus, conditionType corev1.PodConditionType) (int, *corev1.PodCondition) {
	if status == nil || status.Conditions == nil {
		return -1, nil
	}
	for i := range status.Conditions {
		if status.Conditions[i].Type == conditionType {
			return i, &status.Conditions[i]
		}
	}
	return -1, nil
}

func updatePodCondition(status *corev1.PodStatus, condition *corev1.PodCondition) {
	conditionIndex, oldCondition := getPodCondition(status, condition.Type)

	if oldCondition == nil {
		status.Conditions = append(status.Conditions, *condition)
		return
	}

	if condition.Status == oldCondition.Status {
		condition.LastTransitionTime = oldCondition.LastTransitionTime
	}

	status.Conditions[conditionIndex] = *condition
}

func checkPod(ctx context.Context, config *rest.Config, pod *corev1.Pod) error {
	containerPort := 8888
	if len(pod.Spec.Containers[0].Ports) > 0 {
		containerPort = int(pod.Spec.Containers[0].Ports[0].ContainerPort)
	}
	probe := pod.Spec.Containers[0].ReadinessProbe
	if probe == nil {
		probe = pod.Spec.Containers[0].LivenessProbe
	}
	path := "/"
	probeTimeout := 30 * time.Second
	if probe != nil {
		probeTimeout = time.Duration(probe.TimeoutSeconds) * time.Second
		if probe.HTTPGet != nil {
			path = probe.HTTPGet.Path
		}
	}
	cli, err := rest.RESTClientFor(config)
	if err != nil {
		return err
	}
	request := cli.Get().
		Namespace(pod.Namespace).
		Resource("pods").
		SubResource("proxy").
		Name(k8sNet.JoinSchemeNamePort("http", pod.Name, strconv.Itoa(containerPort))).
		Suffix(path)
	ctx, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()
	result := request.Do(ctx)
	if result.Error() != nil {
		return result.Error()
	}
	var statusCode int
	result.StatusCode(&statusCode)
	if statusCode != 200 {
		return errors.Errorf("unexpected status code %d", statusCode)
	}
	return nil
}
