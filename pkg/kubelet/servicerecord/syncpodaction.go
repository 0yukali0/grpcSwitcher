package servicerecord

import (
	v1 "k8s.io/api/core/v1"
)

// PreCheck check label
func (s *SockRecord) PreCheck(pod *v1.Pod) {
	s.PodSwitch = false
	runtimeName, runtimeLabelUse := GetV1PodLabelWithName(pod)
	if runtimeLabelUse {
		//klog.Infof("PreCheck:runtime label:%s!", runtimeName)
		runtimeServiceExist := s.IsExistedService(runtimeName)
		if runtimeServiceExist {
			s.TargetRuntimeName = runtimeName
			s.PodSwitch = true
			//s.TargetRuntime = s.Service[runtimeName].Runtime
			//s.TargetImage = s.Service[runtimeName].Image
			//klog.Infof("PreCheck:active-%s!", runtimeName)
		}
	}
}

// SwitchReset reset condition
func (s *SockRecord) SwitchReset() {
	s.TargetRuntimeName = ""
	s.PodSwitch = false
	//klog.Infof("SwitchReset:reset!")
}
