// Package spok renders audio spectrograms with ffmpeg. It is built to be
// embedded: construct a Service with New, then either call Render directly or
// mount Service.Handler() on your own http.ServeMux.
package spok

import (
	"fmt"
	"log/slog"
	"runtime"
	"time"
)

const (
	defaultTimeout   = 30 * time.Second
	defaultMaxUpload = 100 << 20 // 100 MiB
)

// Config controls a Service. Pass it to New; every zero-valued field is replaced
// by a sane default, so Config{} is valid and yields a working Service.
//
// Fields fall into three groups: deployment (binary paths, temp dir, logger),
// resource limits that protect the host process (timeout, concurrency, upload
// size), and rendering (defaults plus per-request override policy).
type Config struct {
	// FFmpegPath and FFprobePath locate the external binaries, as bare names
	// resolved via $PATH or as absolute paths. Defaults: "ffmpeg"/"ffprobe".
	FFmpegPath  string
	FFprobePath string

	// TempDir is where intermediate files are written. Default: os.TempDir().
	TempDir string

	// RenderTimeout bounds a single render (probe + ffmpeg). Default: 30s.
	RenderTimeout time.Duration

	// MaxConcurrent caps simultaneous renders. ffmpeg is CPU-bound, so this is
	// the main lever protecting the host. Default: runtime.GOMAXPROCS(0).
	MaxConcurrent int

	// MaxUploadBytes rejects larger uploads in the HTTP handler. A negative
	// value disables the check. Default: 100 MiB.
	MaxUploadBytes int64

	// Defaults are the render options applied when a caller/request omits them.
	Defaults RenderOptions

	// AllowRequestOverrides lets HTTP callers change render options via query
	// params (?w=&h=&color=...). When false, every render uses Defaults.
	// Default: false (locked down), which suits an embedded instance.
	AllowRequestOverrides bool

	// Limits bound per-request overrides. Ignored unless AllowRequestOverrides.
	Limits RenderLimits

	// CacheSize is the maximum number of rendered spectrograms retained in an
	// in-memory LRU keyed by SHA-256(input)+RenderOptions. 0 disables caching
	// (the default). Identical uploads with the same resolved options skip
	// ffmpeg on a hit.
	CacheSize int

	// Logger receives structured logs. Default: slog.Default().
	Logger *slog.Logger
}

func (c Config) withDefaults() Config {
	if c.FFmpegPath == "" {
		c.FFmpegPath = "ffmpeg"
	}
	if c.FFprobePath == "" {
		c.FFprobePath = "ffprobe"
	}
	if c.RenderTimeout <= 0 {
		c.RenderTimeout = defaultTimeout
	}
	if c.MaxConcurrent <= 0 {
		c.MaxConcurrent = runtime.GOMAXPROCS(0)
	}
	if c.MaxUploadBytes == 0 {
		c.MaxUploadBytes = defaultMaxUpload
	}
	if c.Logger == nil {
		c.Logger = slog.Default()
	}
	c.Defaults = c.Defaults.withDefaults()
	c.Limits = c.Limits.withDefaults()
	return c
}

func (c Config) validate() error {
	if err := c.Defaults.validate(); err != nil {
		return fmt.Errorf("defaults: %w", err)
	}
	if err := c.Limits.validate(); err != nil {
		return fmt.Errorf("limits: %w", err)
	}
	return nil
}
