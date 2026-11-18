// Command spok is a thin HTTP wrapper around the spok package. It is a consumer
// of the library like any other embedder; all real logic lives in the package.
package main

import (
	"context"
	"errors"
	"flag"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/slouuuu/spok"
)

func main() {
	// Flags override env vars, which override built-in defaults.
	addr := flagEnvStr("addr", "SPOK_ADDR", ":9090", "listen address")
	ffmpeg := flagEnvStr("ffmpeg", "SPOK_FFMPEG", "ffmpeg", "ffmpeg binary (name or path)")
	ffprobe := flagEnvStr("ffprobe", "SPOK_FFPROBE", "ffprobe", "ffprobe binary (name or path)")
	timeout := flag.Duration("timeout", 30*time.Second, "per-render timeout")
	maxConc := flag.Int("max-concurrent", 0, "max simultaneous renders (0 = NumCPU)")
	maxUpload := flag.Int64("max-upload", 100<<20, "max upload bytes (-1 = unlimited)")
	allowOvr := flag.Bool("allow-overrides", false, "allow ?w=&h=&color=... query overrides")
	cacheSize := flag.Int("cache-size", 0, "LRU cache entries for identical uploads (0 = disabled)")

	// Default render settings.
	width := flag.Int("width", 1024, "default spectrogram width")
	height := flag.Int("height", 512, "default spectrogram height")
	color := flag.String("color", "intensity", "default color mode")
	scale := flag.String("scale", "log", "default scale")
	mode := flag.String("mode", "combined", "default channel mode (combined|separate)")
	win := flag.String("win", "hann", "default window function")
	gain := flag.Float64("gain", 1, "default gain")
	legend := flag.Bool("legend", false, "draw ffmpeg's axis legend")
	overlay := flag.Bool("overlay", true, "draw codec/bitrate label on the image")
	flag.Parse()

	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))

	svc, err := spok.New(spok.Config{
		FFmpegPath:            *ffmpeg,
		FFprobePath:           *ffprobe,
		RenderTimeout:         *timeout,
		MaxConcurrent:         *maxConc,
		MaxUploadBytes:        *maxUpload,
		AllowRequestOverrides: *allowOvr,
		CacheSize:             *cacheSize,
		Logger:                logger,
		Defaults: spok.RenderOptions{
			Width:   *width,
			Height:  *height,
			Color:   spok.ColorMode(*color),
			Scale:   spok.ScaleMode(*scale),
			Channel: spok.ChannelMode(*mode),
			WinFunc: spok.WinFunc(*win),
			Gain:    *gain,
			Legend:  *legend,
			Overlay: *overlay,
		},
	})
	if err != nil {
		logger.Error("invalid configuration", "err", err)
		os.Exit(1)
	}

	srv := &http.Server{
		Addr:              *addr,
		Handler:           svc.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       *timeout + 15*time.Second,
		WriteTimeout:      *timeout + 15*time.Second,
		IdleTimeout:       60 * time.Second,
	}

	go func() {
		logger.Info("spok listening", "addr", *addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("server error", "err", err)
			os.Exit(1)
		}
	}()

	// Graceful shutdown on SIGINT/SIGTERM.
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	<-ctx.Done()
	logger.Info("shutting down")

	shutCtx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutCtx); err != nil {
		logger.Error("shutdown error", "err", err)
	}
}

func flagEnvStr(name, env, def, usage string) *string {
	if v := os.Getenv(env); v != "" {
		def = v
	}
	return flag.String(name, def, usage)
}
