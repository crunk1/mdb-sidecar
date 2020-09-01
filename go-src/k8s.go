package main

import (
	"encoding/json"
	"reflect"
	"sync"

	"github.com/pkg/errors"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

var (
	k8sCS   *kubernetes.Clientset
	k8sCSMx sync.Mutex
)

func k8sGetClientSet() *kubernetes.Clientset {
	k8sCSMx.Lock()
	defer k8sCSMx.Unlock()
	if k8sCS == nil {
		rCfg := &rest.Config{}
		rCfg.Host = "https://10.0.0.1"
		rCfg.BearerTokenFile = "/var/run/secrets/kubernetes.io/serviceaccount/token"
		rCfg.TLSClientConfig.CAFile = "/var/run/secrets/kubernetes.io/serviceaccount/ca.crt"
		k8sCS = kubernetes.NewForConfigOrDie(rCfg)
	}
	return k8sCS
}

// k8sGetPods gets the pods running in the mongo service.
// Does not support paging at this time, so this will break if service has a lot of pods.
func k8sGetPods() ([]v1.Pod, error) {
	s, err := k8sGetService()
	if err != nil {
		return nil, err
	}

	ls := labels.SelectorFromSet(s.Spec.Selector).String()
	ps, err := k8sGetClientSet().CoreV1().Pods(cfg.NS).List(metav1.ListOptions{LabelSelector: ls})
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return ps.Items, nil
}

// k8sGetService gets the mongo statefulset's headless service.
func k8sGetService() (*v1.Service, error) {
	s, err := k8sGetClientSet().CoreV1().Services(cfg.NS).Get(cfg.RSSvc, metav1.GetOptions{})
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return s, nil
}

func k8sAddPodLabel(pod *v1.Pod, key string, value string) error {
	if pod.Labels[key] == value {
		return nil
	}

	const patchType = types.JSONPatchType
	type patch struct {
		Op    string            `json:"op"`
		Path  string            `json:"path"`
		Value map[string]string `json:"value"`
	}

	newLabels := map[string]string{}
	for k, v := range pod.Labels {
		newLabels[k] = v
	}
	newLabels[key] = value
	if reflect.DeepEqual(newLabels, pod.Labels) {
		return nil // No change needed
	}
	p := &patch{Op: "replace", Path: "/metadata/labels", Value: newLabels}
	data, _ := json.Marshal(p)
	newPod, err := k8sGetClientSet().CoreV1().Pods(cfg.NS).Patch(pod.Name, patchType, data)
	if err != nil {
		return errors.WithStack(err)
	}
	*pod = *newPod
	return nil
}

func k8sRemovePodLabel(pod *v1.Pod, key string) error {
	if _, ok := pod.Labels[key]; !ok {
		return nil
	}

	const patchType = types.JSONPatchType
	type patch struct {
		Op    string            `json:"op"`
		Path  string            `json:"path"`
		Value map[string]string `json:"value"`
	}

	newLabels := map[string]string{}
	for k, v := range pod.Labels {
		newLabels[k] = v
	}
	delete(newLabels, key)
	if reflect.DeepEqual(newLabels, pod.Labels) {
		return nil // No change needed
	}
	p := &patch{Op: "replace", Path: "/metadata/labels", Value: newLabels}
	data, _ := json.Marshal(p)
	newPod, err := k8sGetClientSet().CoreV1().Pods(cfg.NS).Patch(pod.Name, patchType, data)
	if err != nil {
		return errors.WithStack(err)
	}
	*pod = *newPod
	return nil
}
