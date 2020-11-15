package servicerecord

import (
	"errors"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	internalapi "k8s.io/cri-api/pkg/apis"
	"k8s.io/klog"
	"k8s.io/kubernetes/pkg/kubelet/remote"
)

const (
	experientRuntimeNum = 3
)

var (
	defaultDuration = metav1.Duration{Duration: time.Duration(10) * time.Second}
)

// GrpcResult save grpc client and active time
type GrpcResult struct {
	Runtime internalapi.RuntimeService
	Image   internalapi.ImageManagerService
	Active  time.Time
}

// SockRecord is a search directory
// It relys on service.json and list.json
type SockRecord struct {
	Sock              []string
	Service           map[string]GrpcResult
	ImageDetail       ImagePullerDetail
	Period            metav1.Duration
	StopCH            chan struct{}
	PodSwitch         bool
	TargetRuntimeName string
	TargetRuntime     internalapi.RuntimeService
	TargetImage       internalapi.ImageManagerService
}

// NewSockRecord return *SockRecord
// Load config and create sockrecord.
// It should be executed when kubelet systemd (re)starts.
func NewSockRecord() (result *SockRecord) {
	result = &SockRecord{
		Sock:              make([]string, 0, experientRuntimeNum),
		Service:           make(map[string]GrpcResult),
		Period:            defaultDuration,
		StopCH:            make(chan struct{}),
		PodSwitch:         false,
		TargetRuntimeName: "",
	}
	klog.Infof("NewSockRecord:Instance creates!")
	// reload existed service
	err := result.LoadService()
	if err != nil {
		klog.Infof("NewSockRecord:Services don't exist and load!")
	} else {
		klog.Infof("NewSockRecord:Services exist and load!")
	}
	//load user excepted service list and update it ot recordservice
	err = result.LoadSock()
	if err != nil {
		klog.Infof("NewSockRecord:List of runtime doesn't exist!")
	} else {
		klog.Infof("NewSockRecord:List of runtime exists!")
	}
	result.LoadDefaultSock()
	klog.Infof("NewSockRecord:Health start!")
	result.HealthStart()
	return result
}

// WrapRuntime return (RuntimeService, actionAffect)
// Wrap internalapi.RuntimeService into InstrumentedRuntimeService.
// It will be called by runtimeSwitch function.
func (s *SockRecord) WrapRuntime(target string) (internalapi.RuntimeService, bool) {
	_, _, err := s.GetImageAndRuntime(target, target, s.Period)
	if err != nil {
		klog.Infof("WrapRuntime:Service doesn't return!")
		return nil, false
	}
	klog.Infof("WrapRuntime:Service exists and return!")
	tmp, flag := s.Service[target], true
	return tmp.Runtime, flag
}

// WrapImage return (ImageManagerService, actionAffect)
// Wrap internalapi.ImageManagerService into InstrumentedImageManagerService.
// It will be called by ImageSwitch function.
func (s *SockRecord) WrapImage(target string) (internalapi.ImageManagerService, bool) {
	_, _, err := s.GetImageAndRuntime(target, target, s.Period)
	if err != nil {
		klog.Infof("WrapImage:Service doesn't return!")
		return nil, false
	}
	klog.Infof("WrapImage:Service exists and return!")
	tmp, flag := s.Service[target], true
	return tmp.Image, flag
}

// SetPeriod return repalceActive
// If timeout is less than requirement that is defined from user
// then repalce original period
func (s *SockRecord) SetPeriod(period metav1.Duration) bool {
	if s.Period.Duration < period.Duration {
		klog.Infof("WrapImage:Period changes!")
		s.Period = period
		return true
	}
	klog.Infof("WrapImage:Period doesn't change!")
	return false
}

// GetImageAndRuntime return (runtime,image, effect)
// In the future,this func will replace func getRuntimeAndImageServices in option pakcage
// check whether question can be satified by existed service list
func (s *SockRecord) GetImageAndRuntime(remoteRuntimeEndpoint string,
	remoteImageEndpoint string,
	runtimeRequestTimeout metav1.Duration) (internalapi.RuntimeService, internalapi.ImageManagerService, error) {
	//For now,we
	same := isSameSock(remoteRuntimeEndpoint, remoteImageEndpoint)
	if !same {
		klog.Infof("GetImageAndRuntime:Sockendpoints is different!")
		errMessage := "Failure:Sockendpoints is different."
		return nil, nil, errors.New(errMessage)
	}
	klog.Infof("GetImageAndRuntime:Sockendpoints is same!")
	//Existed sock searching
	requestSock := remoteRuntimeEndpoint
	existed := s.IsExistedService(requestSock)
	//Unkown service request
	if !existed {
		rs, is, err := bothService(requestSock, runtimeRequestTimeout)
		if err != nil {
			klog.Infof("GetImageAndRuntime:GetBothService ERROR!")
			return nil, nil, err
		}
		s.addService(requestSock, rs, is, runtimeRequestTimeout)
		return rs, is, nil
	}
	klog.Infof("GetImageAndRuntime:Service exists!")
	//Timeout check
	targetService := s.Service[requestSock]
	active := targetService.Active
	period := s.Period
	ok := isLegelService(period, runtimeRequestTimeout, active)
	if !ok {
		klog.Infof("GetImageAndRuntime:Service doesn't satify requirement!")
		rs, is, err := bothService(requestSock, runtimeRequestTimeout)
		if err != nil {
			klog.Infof("GetImageAndRuntime:Service recreates fail!")
			return nil, nil, err
		}
		klog.Infof("GetImageAndRuntime:Service recreates!")
		s.addService(requestSock, rs, is, runtimeRequestTimeout)
	}
	klog.Infof("GetImageAndRuntime:Requirement of service is return!")
	service := s.Service[requestSock]
	return service.Runtime, service.Image, nil
}

// private func

// bothService return client of runtime and image
// get runtime and image service
func bothService(runtimeName string, timeoutPeriod metav1.Duration) (internalapi.RuntimeService, internalapi.ImageManagerService, error) {
	rs, err := remote.NewRemoteRuntimeService(runtimeName, timeoutPeriod.Duration)
	if err != nil {
		klog.Infof("bothService:Runtime client ERROR!")
		return nil, nil, err
	}
	is, err := remote.NewRemoteImageService(runtimeName, timeoutPeriod.Duration)
	if err != nil {
		klog.Infof("bothService:Image client ERROR!")
		return nil, nil, err
	}
	klog.Infof("bothService:Runtime and image client return!")
	return rs, is, nil
}

// addService return boolean
// it adds or replaces a complete service to sockrecord
func (s *SockRecord) addService(
	sock string,
	runtime internalapi.RuntimeService,
	image internalapi.ImageManagerService,
	period metav1.Duration) {
	klog.Infof("addService:Service add!")
	t := time.Now()
	s.Sock = append(s.Sock, sock)
	result := GrpcResult{
		Runtime: runtime,
		Image:   image,
		Active:  t,
	}
	s.Service[sock] = result
	s.SetPeriod(period)
}
