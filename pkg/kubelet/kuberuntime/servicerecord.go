package kuberuntime

import (
	"encoding/json"
	"errors"
	"io/ioutil"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/flowcontrol"
	internalapi "k8s.io/cri-api/pkg/apis"
	"k8s.io/klog"
	"k8s.io/kubernetes/pkg/kubelet/remote"
)

const (
	configPath          = "/etc/kubernetes/exceptRuntime.json"
	jsonPath            = "/etc/kubernetes/service.json"
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

// ImagePullerDetail save detail
type ImagePullerDetail struct {
	recorder     record.EventRecorder
	imageBackOff *flowcontrol.Backoff
	serialized   bool
	qps          float32
	burst        int
}

// SockRecord is a search directory
// It relys on service.json and list.json
type SockRecord struct {
	Sock        []string
	Service     map[string]GrpcResult
	ImageDetail ImagePullerDetail
	Period      metav1.Duration
	StopCH      chan struct{}
}

// NewSockRecord return *SockRecord
// Load config and create sockrecord.
// It should be executed when kubelet systemd (re)starts.
func NewSockRecord() (result *SockRecord) {
	result = &SockRecord{
		Sock:    make([]string, 0, experientRuntimeNum),
		Service: make(map[string]GrpcResult),
		Period:  defaultDuration,
		StopCH:  make(chan struct{}),
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

// LoadDefaultSock return nothing
// It load default runtime for experment
func (s *SockRecord) LoadDefaultSock() {
	except := []string{
		"unix:///var/run/crio/crio.sock",
		"unix:///run/containerd/containerd.sock",
		"unix:///var/run/docker.sock",
	}
	for _, target := range except {
		find := s.IsExistedService(target)
		if !find {
			klog.Infof("LoadDefaultSock:Add %s", target)
			_, _, err := s.GetImageAndRuntime(target, target, s.Period)
			if err != nil {
				klog.Infof("LoadDefaultSock:Add unkown service into SockRecord fail!")
			}
		}
	}
}

// LoadSock return error
// Load runtime list
// New runtime will be joined into SockRecord
func (s *SockRecord) LoadSock() error {
	file, err := ioutil.ReadFile(jsonPath)
	if err != nil {
		klog.Infof("LoadSock:File of list doesn't exist!")
		return err
	}
	klog.Infof("LoadSock:List of runtime exists!")

	except := make([]string, 0, experientRuntimeNum)
	err = json.Unmarshal([]byte(file), &except)
	if err != nil {
		klog.Infof("LoadSock:Loading fails!")
		return err
	}
	klog.Infof("LoadSock:Loading success!")
	for _, target := range except {
		find := s.IsExistedService(target)
		if !find {
			klog.Infof("LoadSock:Unkown runtime found!")
			klog.Infof("LoadSock:Add %s", target)
			_, _, err := s.GetImageAndRuntime(target, target, s.Period)
			if err != nil {
				klog.Infof("LoadSock:Add unkown service into SockRecord fail!")
			}
		}
	}
	return nil
}

// LoadService return error
// Load and update sockrecord
func (s *SockRecord) LoadService() error {
	file, err := ioutil.ReadFile(jsonPath)
	if err != nil {
		klog.Infof("LoadService:File of Services doesn't exist!")
		return err
	}
	klog.Infof("LoadService:File of Services exist!")
	err = json.Unmarshal([]byte(file), s)
	if err != nil {
		klog.Infof("LoadService:Services Loading fails!")
		return err
	}
	klog.Infof("LoadService:Services are loaded into SockRecord!")
	return nil
}

// SaveService return error
// Save running sockrecord as json
// It is executed by updater
func (s *SockRecord) SaveService() error {
	file, err := json.MarshalIndent(*s, "", " ")
	if err != nil {
		klog.Infof("SaveService:Services convertion fails!")
		return err
	}
	klog.Infof("SaveService:Services convertion success!")
	err = ioutil.WriteFile(jsonPath, file, 0644)
	if err != nil {
		klog.Infof("SaveService:Services written into file fail!")
		return err
	}
	klog.Infof("SaveService:Services are written into file!")
	return nil
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
	return newInstrumentedRuntimeService(tmp.Runtime), flag
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
	return newInstrumentedImageManagerService(tmp.Image), flag
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

// GetImagePullerDetail return all arguments
// It records arg of imagePuller
func (s *SockRecord) GetImagePullerDetail() (recorder record.EventRecorder,
	imageBackOff *flowcontrol.Backoff,
	serialized bool,
	qps float32,
	burst int) {
	klog.Infof("GetImagePullerDetail:Arg return!")
	detail := s.ImageDetail
	recorder = detail.recorder
	imageBackOff = detail.imageBackOff
	serialized = detail.serialized
	qps = detail.qps
	burst = detail.burst
	return recorder, imageBackOff, serialized, qps, burst
}

// HealthStart return start boolean
func (s *SockRecord) HealthStart() bool {
	klog.Infof("HealthStart:Health check start!")
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
	klog.Infof("HealthStart:Health check stop!")
	s.StopCH <- struct{}{}
	return true
}
