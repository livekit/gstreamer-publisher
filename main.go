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
	"fmt"
	"os"
	"strings"

	"github.com/urfave/cli/v2"

	"github.com/livekit/protocol/logger"
	lksdk "github.com/livekit/server-sdk-go/v2"
)

func main() {
	app := &cli.App{
		Name:      "gstreamer-publisher",
		Usage:     "Publish video/audio from a GStreamer pipeline to LiveKit",
		Version:   "0.1.0",
		UsageText: "gstreamer-publisher --url <url> --token <token> -- <gst-launch parameters>",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "url",
				Usage:   "url to LiveKit instance",
				EnvVars: []string{"LIVEKIT_URL"},
				Value:   "http://localhost:7880",
			},
			&cli.StringFlag{
				Name:     "token",
				Usage:    "access token for authentication. canPublish permission is required",
				Required: true,
			},
			&cli.BoolFlag{
				Name: "verbose",
			},
		},
		Action: func(c *cli.Context) error {
			publisher := NewPublisher(PublisherParams{
				URL:            c.String("url"),
				Token:          c.String("token"),
				PipelineString: strings.Join(c.Args().Slice(), " "),
			})
			return publisher.Start()
		},
	}

	logger.InitFromConfig(&logger.Config{Level: "info"}, "gstreamer-publisher")
	lksdk.SetLogger(logger.GetLogger())
	if err := app.Run(os.Args); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
