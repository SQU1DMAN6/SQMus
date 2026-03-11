package vst

import (
	"errors"
	"fmt"

	"sqmus/internal/compiler"
)

// Options configures VST rendering/playback.
type Options struct {
	PluginPath string
	PresetPath string
	SampleRate int
	BufferSize int
}

// ErrUnavailable indicates that VST support is not embedded in this build.
var ErrUnavailable = errors.New("vst support is not embedded in this build")

// Available reports whether VST support is embedded.
func Available() bool {
	return false
}

// RenderWAV renders a score using an embedded VST host.
func RenderWAV(_ *compiler.Score, _ Options, _ string) error {
	return fmt.Errorf("render via vst: %w", ErrUnavailable)
}

// Play renders and plays a score using an embedded VST host.
func Play(_ *compiler.Score, _ Options) error {
	return fmt.Errorf("play via vst: %w", ErrUnavailable)
}
