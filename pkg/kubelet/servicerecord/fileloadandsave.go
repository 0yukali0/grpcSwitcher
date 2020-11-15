package servicerecord

import (
	"encoding/json"
	"io/ioutil"

	"k8s.io/klog"
)

const (
	configPath = "/etc/kubernetes/exceptRuntime.json"
	jsonPath   = "/etc/kubernetes/service.json"
)

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
