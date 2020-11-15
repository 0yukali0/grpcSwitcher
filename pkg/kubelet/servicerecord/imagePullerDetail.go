package servicerecord

import (
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/flowcontrol"
	"k8s.io/klog"
)

// ImagePullerDetail save detail
type ImagePullerDetail struct {
	Recorder     record.EventRecorder
	ImageBackOff *flowcontrol.Backoff
	Serialized   bool
	RequestQPS   float32
	Burst        int
	PullerCreate bool
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
	recorder = detail.Recorder
	imageBackOff = detail.ImageBackOff
	serialized = detail.Serialized
	qps = detail.RequestQPS
	burst = detail.Burst
	return recorder, imageBackOff, serialized, qps, burst
}
