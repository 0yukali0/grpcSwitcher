package servicerecord

import (
	"strings"

	v1 "k8s.io/api/core/v1"
)

const (
	runtimeLabel = "userruntime"
)

// GetV1PodLabelWithName return (runtimeName, effect)
// This func get label from pod and return
func GetV1PodLabelWithName(pod *v1.Pod) (string, bool) {
	meta := pod.ObjectMeta
	label, find := meta.Labels[runtimeLabel]
	if find {
		old := "_"
		replace := "/"
		label = "unix:///" + strings.Replace(label, old, replace, -1)
		return label, true
	}
	return "", false
}
