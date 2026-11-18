package spok

import (
	"fmt"
	"strings"
)

// Named, validated enums mirror ffmpeg's showspectrumpic options, so an invalid
// value is rejected up front instead of producing a confusing ffmpeg error.
type (
	ColorMode   string
	ScaleMode   string
	ChannelMode string
	WinFunc     string
)

var (
	colorModes = map[ColorMode]struct{}{
		"channel": {}, "intensity": {}, "rainbow": {}, "moreland": {},
		"nebulae": {}, "fire": {}, "fiery": {}, "fruit": {}, "cool": {},
		"magma": {}, "green": {}, "viridis": {}, "plasma": {}, "cividis": {},
		"terrain": {},
	}
	scaleModes = map[ScaleMode]struct{}{
		"lin": {}, "sqrt": {}, "cbrt": {}, "log": {}, "4thrt": {}, "5thrt": {},
	}
	channelModes = map[ChannelMode]struct{}{
		"combined": {}, "separate": {},
	}
	winFuncs = map[WinFunc]struct{}{
		"rect": {}, "bartlett": {}, "hann": {}, "hanning": {}, "hamming": {},
		"blackman": {}, "welch": {}, "flattop": {}, "bharris": {}, "bnuttall": {},
		"bhann": {}, "sine": {}, "nuttall": {}, "lanczos": {}, "gauss": {},
		"tukey": {}, "dolph": {}, "cauchy": {}, "parzen": {}, "poisson": {},
		"bohman": {},
	}
)

// RenderOptions are the per-render visual settings. Numeric/enum zero values are
// filled by withDefaults; the boolean fields are taken literally, so set them
// explicitly in Config.Defaults (Overlay in particular).
type RenderOptions struct {
	Width   int
	Height  int
	Color   ColorMode
	Scale   ScaleMode
	Channel ChannelMode // ffmpeg "mode": combined or separate
	WinFunc WinFunc
	Gain    float64
	Legend  bool // ffmpeg's built-in axis legend
	Overlay bool // draw the codec/sample-rate/bitrate label on the image
}

func (r RenderOptions) withDefaults() RenderOptions {
	if r.Width == 0 {
		r.Width = 1024
	}
	if r.Height == 0 {
		r.Height = 512
	}
	if r.Color == "" {
		r.Color = "intensity"
	}
	if r.Scale == "" {
		r.Scale = "log"
	}
	if r.Channel == "" {
		r.Channel = "combined"
	}
	if r.WinFunc == "" {
		r.WinFunc = "hann"
	}
	if r.Gain == 0 {
		r.Gain = 1
	}
	return r
}

func (r RenderOptions) validate() error {
	if r.Width <= 0 || r.Height <= 0 {
		return fmt.Errorf("width and height must be positive")
	}
	if _, ok := colorModes[r.Color]; !ok {
		return fmt.Errorf("unknown color %q", r.Color)
	}
	if _, ok := scaleModes[r.Scale]; !ok {
		return fmt.Errorf("unknown scale %q", r.Scale)
	}
	if _, ok := channelModes[r.Channel]; !ok {
		return fmt.Errorf("unknown channel/mode %q", r.Channel)
	}
	if _, ok := winFuncs[r.WinFunc]; !ok {
		return fmt.Errorf("unknown win_func %q", r.WinFunc)
	}
	return nil
}

// RenderLimits bound per-request overrides so callers can't ask for a
// 100000x100000 render and exhaust memory.
type RenderLimits struct {
	MinWidth, MaxWidth   int
	MinHeight, MaxHeight int
}

func (l RenderLimits) withDefaults() RenderLimits {
	if l.MinWidth == 0 {
		l.MinWidth = 64
	}
	if l.MaxWidth == 0 {
		l.MaxWidth = 4096
	}
	if l.MinHeight == 0 {
		l.MinHeight = 32
	}
	if l.MaxHeight == 0 {
		l.MaxHeight = 4096
	}
	return l
}

func (l RenderLimits) validate() error {
	if l.MaxWidth < l.MinWidth || l.MaxHeight < l.MinHeight {
		return fmt.Errorf("max must be >= min")
	}
	return nil
}

func (l RenderLimits) clamp(r RenderOptions) RenderOptions {
	r.Width = clampInt(r.Width, l.MinWidth, l.MaxWidth)
	r.Height = clampInt(r.Height, l.MinHeight, l.MaxHeight)
	return r
}

func clampInt(v, lo, hi int) int {
	switch {
	case v < lo:
		return lo
	case v > hi:
		return hi
	default:
		return v
	}
}

func buildFilter(r RenderOptions, label string) string {
	var b strings.Builder
	fmt.Fprintf(&b,
		"showspectrumpic=s=%dx%d:mode=%s:color=%s:scale=%s:win_func=%s:gain=%g:legend=%t",
		r.Width, r.Height, r.Channel, r.Color, r.Scale, r.WinFunc, r.Gain, r.Legend)
	if r.Overlay && label != "" {
		fmt.Fprintf(&b,
			",drawtext=text='%s':x=10:y=10:fontsize=18:fontcolor=white:borderw=2:bordercolor=black",
			escapeDrawtext(label))
	}
	return b.String()
}

// escapeDrawtext escapes characters special to ffmpeg's filter/drawtext syntax.
// The label is built from a constrained set (codec, kHz, kbps), so this is
// defensive rather than load-bearing.
func escapeDrawtext(s string) string {
	return strings.NewReplacer(
		`\`, `\\`,
		`:`, `\:`,
		`'`, `\'`,
		`%`, `\%`,
	).Replace(s)
}
