// Copyright 2024 LiveKit, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"errors"
	"fmt"
	"io"
	"sync/atomic"
	"time"

	"github.com/go-gst/go-gst/gst"
	"github.com/go-gst/go-gst/gst/app"
	"github.com/pion/rtcp"
	"github.com/pion/webrtc/v4"
	"github.com/pion/webrtc/v4/pkg/media"

	"github.com/livekit/protocol/logger"
	lksdk "github.com/livekit/server-sdk-go/v2"
)

const minKeyframeRequestInterval = 500 * time.Millisecond

type publisherTrack struct {
	track                 *lksdk.LocalTrack
	sink                  *app.Sink
	mimeType              string
	publication           *lksdk.LocalTrackPublication
	isEnded               atomic.Bool
	onEOS                 func()
	lastKeyframeRequestNs atomic.Int64
}

func createPublisherTrack(mimeType string) (*publisherTrack, error) {
	webrtcMime := ""
	if mimeType == "audio/x-opus" {
		webrtcMime = webrtc.MimeTypeOpus
	} else if mimeType == "video/x-h264" {
		webrtcMime = webrtc.MimeTypeH264
	} else if mimeType == "video/x-vp8" {
		webrtcMime = webrtc.MimeTypeVP8
	} else if mimeType == "video/x-vp9" {
		webrtcMime = webrtc.MimeTypeVP9
	} else if mimeType == "video/x-av1" {
		webrtcMime = webrtc.MimeTypeAV1
	} else {
		return nil, fmt.Errorf("unsupported mime type: %v", mimeType)
	}

	sink, err := app.NewAppSink()
	if err != nil {
		return nil, err
	}

	pt := &publisherTrack{
		mimeType: webrtcMime,
		sink:     sink,
	}
	sink.SetCallbacks(&app.SinkCallbacks{
		EOSFunc:       pt.handleEOS,
		NewSampleFunc: pt.handleSample,
	})

	if mimeType == "video/x-h264" {
		sink.SetCaps(gst.NewCapsFromString("video/x-h264,stream-format=byte-stream"))
	}

	pt.track, err = lksdk.NewLocalTrack(
		webrtc.RTPCodecCapability{MimeType: webrtcMime},
		lksdk.WithRTCPHandler(pt.onRTCP),
	)
	if err != nil {
		return nil, err
	}
	return pt, nil
}

func (t *publisherTrack) IsEnded() bool {
	return t.isEnded.Load()
}

// callback function when EOS is received
func (t *publisherTrack) handleEOS(_ *app.Sink) {
	t.isEnded.Store(true)
	if t.onEOS != nil {
		t.onEOS()
	}
}

// callback function when new sample is ready
func (t *publisherTrack) handleSample(sink *app.Sink) gst.FlowReturn {
	s := sink.PullSample()
	if s == nil {
		return gst.FlowEOS
	}

	buffer := s.GetBuffer()
	if buffer == nil {
		return gst.FlowError
	}

	segment := s.GetSegment()
	if segment == nil {
		return gst.FlowError
	}

	duration := buffer.Duration()
	// pts := buffer.PresentationTimestamp()
	// ts := time.Duration(segment.ToRunningTime(gst.FormatTime, uint64(pts)))

	err := t.track.WriteSample(media.Sample{
		Data:     buffer.Bytes(),
		Duration: time.Duration(duration),
	}, nil)

	switch {
	case err == nil:
		return gst.FlowOK
	case errors.Is(err, io.EOF):
		return gst.FlowEOS
	default:
		return gst.FlowError
	}
}

func (t *publisherTrack) onRTCP(packet rtcp.Packet) {
	switch packet.(type) {
	case *rtcp.PictureLossIndication, *rtcp.FullIntraRequest:
		t.forceKeyframe()
	}
}

func (t *publisherTrack) forceKeyframe() {
	if t.mimeType == webrtc.MimeTypeOpus {
		return
	}

	now := time.Now().UnixNano()
	last := t.lastKeyframeRequestNs.Load()
	if now-last < int64(minKeyframeRequestInterval) {
		return
	}
	if !t.lastKeyframeRequestNs.CompareAndSwap(last, now) {
		return
	}

	s := gst.NewStructure("GstForceKeyUnit")
	_ = s.SetValue("all-headers", true)
	ev := gst.NewCustomEvent(gst.EventTypeCustomUpstream, s)

	pad := t.sink.GetStaticPad("sink")
	if pad == nil {
		return
	}

	// PushEvent forwards the event to the pad's peer. For the appsink's sink
	// pad, the peer is the src pad of the upstream element, so the upstream
	// force-key-unit event travels upstream toward the encoder. SendEvent
	// would be rejected by GStreamer as "wrong direction" on a sink pad.
	if ok := pad.PushEvent(ev); !ok {
		logger.Warnw("force-keyframe event was not handled by pipeline", nil)
	}
}
