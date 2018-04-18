package stub

import (
	"fmt"
	"reflect"

	"github.com/droot/memcached-operator/pkg/apis/memcached/v1alpha1"

	"github.com/coreos/operator-sdk/pkg/sdk/action"
	"github.com/coreos/operator-sdk/pkg/sdk/handler"
	"github.com/coreos/operator-sdk/pkg/sdk/query"
	"github.com/coreos/operator-sdk/pkg/sdk/types"
	"github.com/sirupsen/logrus"
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
)

func NewHandler() handler.Handler {
	return &Handler{}
}

type Handler struct {
	// Fill me
}

func (h *Handler) Handle(ctx types.Context, event types.Event) error {
	switch o := event.Object.(type) {
	case *v1alpha1.Memcached:
		memcached := o
		// If owner references are set appropriately, we don't have to do
		// anything specific. Everything below this CR in the owner chain will
		// get deleted automatically.
		if event.Deleted {
			return nil
		}
		return syncMemcached(memcached)
	case *v1.Pod:
		logrus.Infof("got a change notification for Pod: %v", o)
		// find the parent to be re-conciled
		pod := o

		memcached, err := getMemcachedControllerOf(pod)
		if err != nil {
			logrus.Infof("error looking up controller object for pod: %v", err)
			return err
		}
		if memcached == nil {
			logrus.Infof("found pod not belonging to memcached service")
			return nil
		}
		err = syncMemcached(memcached)
		if err != nil {
			logrus.Errorf("error syncing memcached: %v", err)
			return err
		}
	}
	return nil
}

func getMemcachedControllerOf(pod *v1.Pod) (*v1alpha1.Memcached, error) {
	ownerRef := metav1.GetControllerOf(pod)
	if ownerRef == nil {
		logrus.Infof("found object with no owner reference, so skipping")
		return nil, nil
	}
	rs := &appsv1.ReplicaSet{
		TypeMeta: metav1.TypeMeta{
			APIVersion: ownerRef.APIVersion,
			Kind:       ownerRef.Kind,
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      ownerRef.Name,
			Namespace: pod.Namespace,
		},
	}
	err := query.Get(rs)
	if err != nil {
		logrus.Infof("error in query replicaset: %v", rs)
		return nil, err
	}

	ownerRef = metav1.GetControllerOf(rs)
	if ownerRef == nil {
		logrus.Infof("found object with no owner reference, so skipping")
		return nil, nil
	}
	dep := &appsv1.Deployment{
		TypeMeta: metav1.TypeMeta{
			APIVersion: ownerRef.APIVersion,
			Kind:       ownerRef.Kind,
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      ownerRef.Name,
			Namespace: pod.Namespace,
		},
	}

	err = query.Get(dep)
	if err != nil {
		logrus.Infof("error in query deployment: %v", dep)
		return nil, err
	}

	ownerRef = metav1.GetControllerOf(dep)
	if ownerRef == nil {
		logrus.Infof("found object with no owner reference, so skipping")
		return nil, nil
	}
	memcached := &v1alpha1.Memcached{
		TypeMeta: metav1.TypeMeta{
			APIVersion: ownerRef.APIVersion,
			Kind:       ownerRef.Kind,
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      ownerRef.Name,
			Namespace: pod.Namespace,
		},
	}

	err = query.Get(memcached)
	if ownerRef == nil {
		logrus.Infof("found object with no owner reference, so skipping")
		return nil, nil
	}
	if err != nil {
		logrus.Infof("error in query memcached: %v", memcached)
		return nil, err
	}

	return memcached, nil
}

func syncMemcached(m *v1alpha1.Memcached) error {
	dep := deploymentForMemcached(m)
	err := action.Create(dep)
	if err != nil && !errors.IsAlreadyExists(err) {
		logrus.Errorf("Failed to create busybox pod : %v", err)
		return err
	}

	// here we should check desired vs real state and adjust
	// Ensure the deployment size is the same as the spec
	err = query.Get(dep)
	if err != nil {
		return fmt.Errorf("failed to get deployment: %v", err)
	}
	size := m.Spec.Size
	if *dep.Spec.Replicas != size {
		dep.Spec.Replicas = &size
		err = action.Update(dep)
		if err != nil {
			return fmt.Errorf("failed to update deployment: %v", err)
		}
	}
	// Update the Memcached status with the pod names
	podList := podList()
	labelSelector := labels.SelectorFromSet(labelsForMemcached(m.Name)).String()
	listOps := &metav1.ListOptions{LabelSelector: labelSelector}
	err = query.List(m.Namespace, podList, query.WithListOptions(listOps))
	if err != nil {
		return fmt.Errorf("failed to list pods: %v", err)
	}
	podNames := getPodNames(podList.Items)
	if !reflect.DeepEqual(podNames, m.Status.Nodes) {
		m.Status.Nodes = podNames
		err := action.Update(m)
		if err != nil {
			return fmt.Errorf("failed to update memcached status: %v", err)
		}
	}

	return nil
}

// labelsForMemcached returns the labels for selecting the resources
// belonging to the given memcached CR name.
func labelsForMemcached(name string) map[string]string {
	return map[string]string{"app": "memcached", "memcached_cr": name}
}

// addOwnerRefToObject appends the desired OwnerReference to the object
func addOwnerRefToObject(obj metav1.Object, ownerRef metav1.OwnerReference) {
	obj.SetOwnerReferences(append(obj.GetOwnerReferences(), ownerRef))
}

// asOwner returns an OwnerReference set as the memcached CR
func asOwner(m *v1alpha1.Memcached) metav1.OwnerReference {
	trueVar := true
	return metav1.OwnerReference{
		APIVersion: m.APIVersion,
		Kind:       m.Kind,
		Name:       m.Name,
		UID:        m.UID,
		Controller: &trueVar,
	}
}

// deploymentForMemcached returns a memcached Deployment object
func deploymentForMemcached(m *v1alpha1.Memcached) *appsv1.Deployment {
	ls := labelsForMemcached(m.Name)
	replicas := m.Spec.Size

	dep := &appsv1.Deployment{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "apps/v1",
			Kind:       "Deployment",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      m.Name,
			Namespace: m.Namespace,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: ls,
			},
			Template: v1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: ls,
				},
				Spec: v1.PodSpec{
					Containers: []v1.Container{{
						Image:   "memcached:1.4.36-alpine",
						Name:    "memcached",
						Command: []string{"memcached", "-m=64", "-o", "modern", "-v"},
						Ports: []v1.ContainerPort{{
							ContainerPort: 11211,
							Name:          "memcached",
						}},
					}},
				},
			},
		},
	}
	addOwnerRefToObject(dep, asOwner(m))
	return dep
}

// podList returns a v1.PodList object
func podList() *v1.PodList {
	return &v1.PodList{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Pod",
			APIVersion: "v1",
		},
	}
}

// getPodNames returns the pod names of the array of pods passed in
func getPodNames(pods []v1.Pod) []string {
	var podNames []string
	for _, pod := range pods {
		podNames = append(podNames, pod.Name)
	}
	return podNames
}
