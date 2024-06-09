# LiveKit GStreamer Publisher

This utility allows you to publish from any GStreamer source to LiveKit.
It parses a gst-launch style pipeline and reads negotiates 

## Prerequisites

- Go 1.22 or later
- GStreamer 1.20 or later

## Install

```bash
go install github.com/livekit/gstreamer-publisher
```

## Usage

To use this utility, you need to [generate an access token](https://docs.livekit.io/home/get-started/authentication/) that includes
the `canPublish` permission.

When constructing pipelines, you would want to end the pipeline with elements that produce H264, VP8, VP9, or Opus.
Do not mux the streams into a container format. GStreamer-publisher will inspect the pipeline and import the raw
streams into LiveKit.

## Examples

### Test stream as H.264

This creates a video and audio from test src, encoding it to H.264 and Opus.

```bash
./gstreamer-publisher --token <token> \
    -- \
    videotestsrc is-live=true ! \
        video/x-raw,width=1280,height=720 ! \
        clockoverlay ! \
        videoconvert ! \
        x264enc tune=zerolatency key-int-max=60 bitrate=2000 \
    audiotestsrc is-live=true ! \
        audioresample ! \
        audioconvert ! \
        opusenc bitrate=64000
```

### Publish from file

The following converts any video file into VP9 and Opus using decodebin3

```bash
./gstreamer-publisher --token <token> \
  -- \
    filesrc location="/path/to/file" ! \
    decodebin3 name=decoder \
    decoder. ! queue ! \
        videoconvert ! \
        videoscale ! \
        video/x-raw,width=1280,height=720 ! \
        vp9enc deadline=1 cpu-used=-6 row-mt=1 tile-columns=3 tile-rows=1 target-bitrate=2000000 keyframe-max-dist=60 \
    decoder. ! queue ! \
        audioconvert ! \
        audioresample ! \
        opusenc bitrate=64000
```
