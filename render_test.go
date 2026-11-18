package spok

import (
	"strings"
	"testing"
)

func TestRenderOptionsWithDefaults(t *testing.T) {
	tests := []struct {
		name string
		in   RenderOptions
		want RenderOptions
	}{
		{
			name: "all zero",
			in:   RenderOptions{},
			want: RenderOptions{
				Width: 1024, Height: 512,
				Color: "intensity", Scale: "log", Channel: "combined",
				WinFunc: "hann", Gain: 1,
			},
		},
		{
			name: "preserves explicit values and booleans",
			in: RenderOptions{
				Width: 800, Height: 400,
				Color: "magma", Scale: "lin", Channel: "separate",
				WinFunc: "hamming", Gain: 2.5,
				Legend: true, Overlay: true,
			},
			want: RenderOptions{
				Width: 800, Height: 400,
				Color: "magma", Scale: "lin", Channel: "separate",
				WinFunc: "hamming", Gain: 2.5,
				Legend: true, Overlay: true,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.in.withDefaults()
			if got != tt.want {
				t.Fatalf("withDefaults() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestRenderOptionsValidate(t *testing.T) {
	valid := RenderOptions{}.withDefaults()
	if err := valid.validate(); err != nil {
		t.Fatalf("valid defaults: %v", err)
	}

	tests := []struct {
		name    string
		mutate  func(*RenderOptions)
		wantSub string
	}{
		{"non-positive size", func(r *RenderOptions) { r.Width = 0 }, "positive"},
		{"bad color", func(r *RenderOptions) { r.Color = "nope" }, "color"},
		{"bad scale", func(r *RenderOptions) { r.Scale = "nope" }, "scale"},
		{"bad channel", func(r *RenderOptions) { r.Channel = "nope" }, "channel"},
		{"bad win_func", func(r *RenderOptions) { r.WinFunc = "nope" }, "win_func"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := valid
			tt.mutate(&opts)
			err := opts.validate()
			if err == nil {
				t.Fatal("expected error")
			}
			if !strings.Contains(err.Error(), tt.wantSub) {
				t.Fatalf("error %q does not contain %q", err, tt.wantSub)
			}
		})
	}
}

func TestRenderLimitsClamp(t *testing.T) {
	limits := RenderLimits{}.withDefaults() // 64..4096 x 32..4096

	tests := []struct {
		name      string
		w, h      int
		wantW, wantH int
	}{
		{"below min", 10, 10, 64, 32},
		{"above max", 99999, 99999, 4096, 4096},
		{"in range", 800, 400, 800, 400},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := limits.clamp(RenderOptions{Width: tt.w, Height: tt.h})
			if got.Width != tt.wantW || got.Height != tt.wantH {
				t.Fatalf("clamp(%d,%d) = %dx%d, want %dx%d",
					tt.w, tt.h, got.Width, got.Height, tt.wantW, tt.wantH)
			}
		})
	}
}

func TestBuildFilter(t *testing.T) {
	opts := RenderOptions{
		Width: 1024, Height: 512,
		Color: "intensity", Scale: "log", Channel: "combined",
		WinFunc: "hann", Gain: 1, Legend: false, Overlay: false,
	}
	want := "showspectrumpic=s=1024x512:mode=combined:color=intensity:scale=log:win_func=hann:gain=1:legend=false"
	if got := buildFilter(opts, "MP3 44.1 kHz"); got != want {
		t.Fatalf("buildFilter without overlay:\n got %q\nwant %q", got, want)
	}

	opts.Overlay = true
	withLabel := buildFilter(opts, "MP3 44.1 kHz")
	if !strings.HasPrefix(withLabel, want+",drawtext=") {
		t.Fatalf("expected drawtext suffix, got %q", withLabel)
	}
	if !strings.Contains(withLabel, "text='MP3 44.1 kHz'") {
		t.Fatalf("missing escaped label in %q", withLabel)
	}

	noLabel := buildFilter(opts, "")
	if strings.Contains(noLabel, "drawtext") {
		t.Fatalf("drawtext present with empty label: %q", noLabel)
	}

	opts.Overlay = false
	noOverlay := buildFilter(opts, "MP3 44.1 kHz")
	if strings.Contains(noOverlay, "drawtext") {
		t.Fatalf("drawtext present with Overlay=false: %q", noOverlay)
	}
}

func TestEscapeDrawtext(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{`a\b`, `a\\b`},
		{`a:b`, `a\:b`},
		{`a'b`, `a\'b`},
		{`a%b`, `a\%b`},
		{`\:'%`, `\\\:\'\%`},
	}
	for _, tt := range tests {
		if got := escapeDrawtext(tt.in); got != tt.want {
			t.Errorf("escapeDrawtext(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}
