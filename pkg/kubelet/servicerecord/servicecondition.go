package servicerecord

import (
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog"
)

// isLegelService return boolean of legal service
// active
//	|------period------|
//		   now
//			|----------requestPeriod-----------|
// => if active + period - now >= requestPeriod ----> legal else illgal.
// => deadline - except = (active + period) - (now + requestPeriod) >= 0
func isLegelService(period, request metav1.Duration, active time.Time) bool {
	except := time.Now().Add(request.Duration)
	deadline := active.Add(period.Duration)
	diff := deadline.Sub(except)
	if diff >= 0 {
		klog.Infof("isLegelService:Period is legal!")
		return true
	}
	klog.Infof("isLegelService:Period is illegal!")
	return false
}

// isSameSock return boolean
// Socks should be same
func isSameSock(runtimeEndpoint string, imageEndpoint string) bool {
	if runtimeEndpoint == imageEndpoint {
		klog.Infof("isSameSock:Socks are same!")
		return true
	}
	klog.Infof("isSameSock:Socks are different!")
	return false
}

// IsExistedService return ServiceExisted
// This func will check target whether it exists
func (s *SockRecord) IsExistedService(remoteRuntimeEndpoint string) bool {
	var sock string
	for _, sock = range s.Sock {
		if sock == remoteRuntimeEndpoint {
			klog.Infof("IsExistedService:Service exists!")
			return true
		}
	}
	klog.Infof("IsExistedService:Service doesn't exist!")
	return false
}
