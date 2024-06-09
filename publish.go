package main

import (
	"errors"
	"os"
	"os/signal"
	"slices"
	"syscall"

	"github.com/go-gst/go-glib/glib"
	"github.com/go-gst/go-gst/gst"

	"github.com/livekit/protocol/livekit"
	"github.com/livekit/protocol/logger"
	lksdk "github.com/livekit/server-sdk-go/v2"
)

var (
	supportedAudioMimeTypes = []string{
		"audio/x-opus",
	}
	supportedVideoMimeTypes = []string{
		"video/x-h264",
		"video/x-vp8",
		"video/x-vp9",
		"video/x-av1",
	}
)

type PublisherParams struct {
	URL            string
	Token          string
	PipelineString string
}

type Publisher struct {
	params     PublisherParams
	pipeline   *gst.Pipeline
	loop       *glib.MainLoop
	videoTrack *publisherTrack
	audioTrack *publisherTrack
	room       *lksdk.Room
}

type elementTarget struct {
	element  *gst.Element
	srcPad   *gst.Pad
	mimeType string
	isAudio  bool
}

func NewPublisher(params PublisherParams) *Publisher {
	return &Publisher{
		params: params,
	}
}

func (p *Publisher) Start() error {
	if err := p.initialize(); err != nil {
		return err
	}

	// TODO: connect at the same time in parallel as spinning up pipeline
	cb := lksdk.NewRoomCallback()
	cb.OnDisconnected = func() {
		// TODO: stop publishing and exit
	}
	p.room = lksdk.NewRoom(cb)
	err := p.room.JoinWithToken(p.params.URL, p.params.Token,
		lksdk.WithAutoSubscribe(false),
	)
	if err != nil {
		return err
	}

	// publish tracks if sinks are set up
	if p.videoTrack != nil {
		pub, err := p.room.LocalParticipant.PublishTrack(p.videoTrack.track, &lksdk.TrackPublicationOptions{
			Source: livekit.TrackSource_CAMERA,
		})
		if err != nil {
			return err
		}
		p.videoTrack.publication = pub
		p.videoTrack.onEOS = func() {
			_ = p.room.LocalParticipant.UnpublishTrack(pub.SID())
		}
	}

	if p.audioTrack != nil {
		pub, err := p.room.LocalParticipant.PublishTrack(p.audioTrack.track, &lksdk.TrackPublicationOptions{
			Source: livekit.TrackSource_MICROPHONE,
		})
		if err != nil {
			return err
		}
		p.audioTrack.publication = pub
		p.audioTrack.onEOS = func() {
			_ = p.room.LocalParticipant.UnpublishTrack(pub.SID())
		}
	}

	if err := p.pipeline.Start(); err != nil {
		return err
	}

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)
	go func() {
		<-sigChan
		p.Stop()
	}()

	p.loop.Run()
	return nil
}

func (p *Publisher) Stop() {
	logger.Infow("stopping publisher..")
	if p.pipeline != nil {
		p.pipeline.BlockSetState(gst.StateNull)
		p.pipeline = nil
	}
	if p.room != nil {
		p.room.Disconnect()
		p.room = nil
	}
	if p.loop != nil {
		p.loop.Quit()
		p.loop = nil
	}
}

func (p *Publisher) messageWatch(msg *gst.Message) bool {
	switch msg.Type() {
	case gst.MessageEOS:
		// EOS received - close and return
		logger.Infow("EOS received, stopping pipeline")
		p.Stop()
		return false

	case gst.MessageError:
		// handle error if possible, otherwise close and return
		logger.Infow("pipeline failure", "error", msg)
		p.Stop()
		return false

	case gst.MessageTag, gst.MessageStateChanged, gst.MessageLatency, gst.MessageAsyncDone, gst.MessageStreamStatus, gst.MessageElement:
		// ignore

	default:
		logger.Debugw(msg.String())
	}

	return true
}

func (p *Publisher) initialize() error {
	if p.pipeline != nil {
		return nil
	}
	gst.Init(nil)
	p.loop = glib.NewMainLoop(glib.MainContextDefault(), false)
	pipeline, err := gst.NewPipelineFromString(p.params.PipelineString)
	if err != nil {
		return err
	}
	pipeline.GetPipelineBus().AddWatch(p.messageWatch)

	// auto-find audio and video elements matching our specs
	targets, err := p.discoverSuitableElements(pipeline)
	if err != nil {
		return err
	}

	if len(targets) == 0 {
		return errors.New("no supported elements found. pipeline needs to include encoded audio or video")
	}

	for _, target := range targets {
		if p.videoTrack != nil && !target.isAudio {
			return errors.New("pipeline has more than one video source")
		} else if p.audioTrack != nil && target.isAudio {
			return errors.New("pipeline has more than one audio source")
		}
		pt, err := createPublisherTrack(target.mimeType)
		if err != nil {
			return err
		}
		if err := pipeline.Add(pt.sink.Element); err != nil {
			return err
		}
		if err := target.element.Link(pt.sink.Element); err != nil {
			return err
		}

		logger.Infow("found source", "mimeType", target.mimeType)
		if target.isAudio {
			p.audioTrack = pt
		} else {
			p.videoTrack = pt
		}
	}

	p.pipeline = pipeline
	return nil
}

func (p *Publisher) discoverSuitableElements(pipeline *gst.Pipeline) ([]elementTarget, error) {
	elements, err := pipeline.GetElements()
	if err != nil {
		return nil, err
	}

	var targets []elementTarget
	for _, e := range elements {
		pads, err := e.GetSrcPads()
		if err != nil {
			return nil, err
		}
		for _, pad := range pads {
			if !pad.IsLinked() {
				caps := pad.GetPadTemplateCaps()
				if caps == nil {
					continue
				}
				structure := caps.GetStructureAt(0)
				mime := structure.Name()
				if slices.Contains(supportedAudioMimeTypes, mime) {
					targets = append(targets, elementTarget{
						element:  e,
						srcPad:   pad,
						mimeType: mime,
						isAudio:  true,
					})
				} else if slices.Contains(supportedVideoMimeTypes, mime) {
					targets = append(targets, elementTarget{
						element:  e,
						srcPad:   pad,
						mimeType: mime,
						isAudio:  false,
					})
				}
			}
		}
	}
	return targets, nil
}
