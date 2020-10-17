package kuberuntime

import (
	"encoding/json"
	"errors"
	"io/ioutil"
	"sort"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	internalapi "k8s.io/cri-api/pkg/apis"
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

type GrpcResult struct {
	Runtime internalapi.RuntimeService
	Image   internalapi.ImageManagerService
	Active  time.Time
}

type SockRecord struct {
	Sock    []string
	Service map[string]GrpcResult
	Period  metav1.Duration
}

/*
*	Load config and create sockrecord.
*	It should be executed when kubelet systemd (re)starts.
 */
func NewSockRecord() (result *SockRecord) {
	result = &SockRecord{
		Sock:    make([]string, 0, experientRuntimeNum),
		Service: make(map[string]GrpcResult),
		Period:  defaultDuration,
	}
	_ = result.LoadService() //reload existed service
	_ = result.LoadSock()    //load user excepted service list and update it ot recordservice
	return result
}

func (s *SockRecord) LoadSock() error {
	file, err := ioutil.ReadFile(jsonPath)
	if err != nil {
		return err
	}
	except := make([]string, 0, experientRuntimeNum)
	err = json.Unmarshal([]byte(file), &except)
	if err != nil {
		return err
	}
	var index int
	for _, target := range except {
		index = sort.SearchStrings(s.Sock, target)
		if index > len(s.Sock) || s.Sock[index] != target {
			return nil
		}
	}
	return nil
}

/*
*	Load and update sockrecord
 */
func (s *SockRecord) LoadService() error {
	file, err := ioutil.ReadFile(jsonPath)
	if err != nil {
		return err
	}
	err = json.Unmarshal([]byte(file), s)
	if err != nil {
		return err
	}
	return nil
}

/*
*	Save running sockrecord as json
*	It is executed by updater
 */
func (s *SockRecord) SaveService() error {
	file, err := json.MarshalIndent(*s, "", " ")
	if err != nil {
		return err
	}
	err = ioutil.WriteFile(jsonPath, file, 0644)
	if err != nil {
		return err
	}
	return nil
}

/*
*	Wrap internalapi.RuntimeService into InstrumentedRuntimeService.
*	It will be called by runtimeSwitch function.
 */
func (s *SockRecord) WrapRuntime(target string) (internalapi.RuntimeService, bool) {
	index, flag := sort.SearchStrings(s.Sock, target), false
	if index <= len(s.Sock) && s.Sock[index] != target {
		return nil, flag
	}
	tmp, flag := s.Service[target], true
	return newInstrumentedRuntimeService(tmp.Runtime), flag
}

/*
*	Wrap internalapi.ImageManagerService into InstrumentedImageManagerService.
*	It will be called by ImageSwitch function.
 */
func (s *SockRecord) WrapImage(target string) (internalapi.ImageManagerService, bool) {
	index, flag := sort.SearchStrings(s.Sock, target), false
	if index <= len(s.Sock) && s.Sock[index] != target {
		return nil, flag
	}
	tmp, flag := s.Service[target], true
	return newInstrumentedImageManagerService(tmp.Image), flag
}

func (s *SockRecord) SetPeriod(period metav1.Duration) bool {
	if s.Period.Duration < period.Duration {
		s.Period = period
		return true
	}
	return false
}

func (s *SockRecord) IsExistedService(remoteRuntimeEndpoint string) bool {
	var sock string
	for _, sock = range s.Sock {
		if sock == remoteRuntimeEndpoint {
			return true
		}
	}
	return false
}

/*
*	In the future,this func will replace func getRuntimeAndImageServices in option pakcage
*	check whether question can be satified by existed service list
 */
func (s *SockRecord) GetImageAndRuntime(remoteRuntimeEndpoint string,
	remoteImageEndpoint string,
	runtimeRequestTimeout metav1.Duration) (internalapi.RuntimeService, internalapi.ImageManagerService, error) {
	//For now,we
	same := isSameSock(remoteRuntimeEndpoint, remoteImageEndpoint)
	if !same {
		return nil, nil, errors.New("Failure:Sockendpoints is different.")
	}

	//Existed sock searching
	requestSock := remoteRuntimeEndpoint
	existed := s.IsExistedService(requestSock)

	//Unkown service request
	if !existed {
		rs, is, err := bothService(requestSock, runtimeRequestTimeout)
		if err != nil {
			return nil, nil, err
		}
		t := time.Now()
		s.addService(requestSock, rs, is, t, runtimeRequestTimeout)
		return rs, is, nil
	}

	//Timeout check
	targetService := s.Service[requestSock]
	active := targetService.Active
	period := s.Period
	ok := isLegelService(period, runtimeRequestTimeout, active)
	if !ok {
		return nil, nil, errors.New("Failure:Existed services don't satify requestion.")
	}

	service := s.Service[requestSock]
	return service.Runtime, service.Image, nil
}

//private
//get runtime and image service
func bothService(runtimeName string, timeoutPeriod metav1.Duration) (internalapi.RuntimeService, internalapi.ImageManagerService, error) {
	rs, err := remote.NewRemoteRuntimeService(runtimeName, timeoutPeriod.Duration)
	if err != nil {
		return nil, nil, err
	}
	is, err := remote.NewRemoteImageService(runtimeName, timeoutPeriod.Duration)
	if err != nil {
		return nil, nil, err
	}
	return rs, is, nil
}

/*
* active
*	|------period------|
*		   now
*			|----------requestPeriod-----------|
*
*	=> if active + period - now >= requestPeriod ----> legal else illgal.
*	=> deadline - except = (active + period) - (now + requestPeriod) >= 0
 */
func isLegelService(period, request metav1.Duration, active time.Time) bool {
	except := time.Now().Add(request.Duration)
	deadline := active.Add(period.Duration)
	diff := deadline.Sub(except)
	if diff >= 0 {
		return true
	}
	return false
}

func isSameSock(runtimeEndpoint string, imageEndpoint string) bool {
	if runtimeEndpoint == imageEndpoint {
		return true
	}
	return false
}

/*
*	update a complete service to sockrecord
 */
func (s *SockRecord) addService(
	sock string,
	runtime internalapi.RuntimeService,
	image internalapi.ImageManagerService,
	t time.Time,
	period metav1.Duration) {
	s.Sock = append(s.Sock, sock)
	result := GrpcResult{
		Runtime: runtime,
		Image:   image,
		Active:  t,
	}
	s.Service[sock] = result
	if s.Period.Duration < period.Duration {
		s.Period = period
	}
}
