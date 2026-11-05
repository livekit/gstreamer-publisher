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
	"context"
	"log/slog"
	"testing"

	"github.com/urfave/cli/v3"
)

func TestVerboseFlag(t *testing.T) {
	tests := []struct {
		name  string
		args  []string
		debug bool
	}{
		{
			name:  "default",
			args:  []string{"gstreamer-publisher", "--token", "token"},
			debug: false,
		},
		{
			name:  "verbose",
			args:  []string{"gstreamer-publisher", "--token", "token", "--verbose"},
			debug: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cmd := newCommand()
			cmd.Action = func(ctx context.Context, _ *cli.Command) error {
				if enabled := slog.Default().Enabled(ctx, slog.LevelDebug); enabled != tc.debug {
					t.Errorf("expected debug logging enabled = %v, got %v", tc.debug, enabled)
				}
				return nil
			}
			if err := cmd.Run(context.Background(), tc.args); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}
