package servicerecord

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/klog"
)

// HealthStart return start boolean
func (s *SockRecord) HealthStart() bool {
	//klog.Infof("HealthStart:Health check start!")
	duration := s.Period
	go wait.Until(func() {
		for runtimeSock := range s.Service {
			_, _, err := s.GetImageAndRuntime(runtimeSock, runtimeSock, duration)
			if err != nil {
				klog.Infof("HealthStart:%s service doesn't update!", runtimeSock)
			}
		}
	}, duration.Duration, s.StopCH)
	return true
}

// HealthRestart reutrn restart boolean
func (s *SockRecord) HealthRestart(period metav1.Duration) bool {
	s.HealthStop()
	s.SetPeriod(period)
	return s.HealthStart()
}

// HealthStop return stop boolean
func (s *SockRecord) HealthStop() bool {
	//klog.Infof("HealthStart:Health check stop!")
	s.StopCH <- struct{}{}
	return true
}
