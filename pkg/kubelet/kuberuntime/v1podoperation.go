package kuberuntime

import (
	v1 "k8s.io/api/core/v1"
)

const (
	runtimeLabel = "runtime"
)

func GetV1PodLabelWithName(pod *v1.Pod) (string, bool) {
	meta := pod.ObjectMeta
	label, find := meta.Labels[runtimeLabel]
	if find {
		return label, true
	}
	return "", false
}

func IsRuntimeSwitch(pod *v1.Pod, bind string) (string, bool) {
	runtime, existed := GetV1PodLabelWithName(pod)
	if !existed {
		return bind, false
	}
	if runtime == bind {
		return bind, false
	}
	return runtime, true
}
