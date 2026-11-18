package spok

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
)

// ErrRender is returned when ffmpeg fails for reasons other than cancellation or
// timeout. The underlying ffmpeg output is logged, not returned, so internal
// detail never leaks to an HTTP client.
var ErrRender = errors.New("spok: render failed")

// Service renders spectrograms. Construct it with New; it is safe for concurrent
// use and bounds its own concurrency per Config.MaxConcurrent.
type Service struct {
	cfg   Config
	sem   chan struct{}
	cache *lruCache
}

// New validates cfg, fills defaults, and returns a ready Service.
func New(cfg Config) (*Service, error) {
	cfg = cfg.withDefaults()
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	return &Service{
		cfg:   cfg,
		sem:   make(chan struct{}, cfg.MaxConcurrent),
		cache: newLRU(cfg.CacheSize),
	}, nil
}

// Render reads audio from src and returns a PNG spectrogram plus the probed
// metadata label (empty if probing failed or opts.Overlay is false). ext is the
// source file extension (with dot) and is used only to hint ffmpeg's demuxer.
//
// Render honors ctx, applies the Service timeout, and respects the concurrency
// limit, blocking until a slot is free or ctx is done.
func (s *Service) Render(ctx context.Context, src io.Reader, ext string, opts RenderOptions) (png []byte, label string, err error) {
	opts = opts.withDefaults()
	if err := opts.validate(); err != nil {
		return nil, "", fmt.Errorf("spok: invalid render options: %w", err)
	}

	// Concurrency gate: acquire a slot or give up if the caller cancels.
	select {
	case s.sem <- struct{}{}:
		defer func() { <-s.sem }()
	case <-ctx.Done():
		return nil, "", ctx.Err()
	}

	ctx, cancel := context.WithTimeout(ctx, s.cfg.RenderTimeout)
	defer cancel()

	// Hash the upload while writing the temp file so cache keys don't require
	// a second pass over the bytes.
	sum := sha256.New()
	inPath, err := s.writeTemp(io.TeeReader(src, sum), "spok-*"+safeExt(ext))
	if err != nil {
		return nil, "", err
	}
	defer os.Remove(inPath)

	var cacheKey string
	if s.cache != nil {
		cacheKey = makeCacheKey(sum.Sum(nil), opts)
		if cached, cachedLabel, ok := s.cache.get(cacheKey); ok {
			return cached, cachedLabel, nil
		}
	}

	if opts.Overlay {
		label = s.probe(ctx, inPath)
	}

	outf, err := os.CreateTemp(s.cfg.TempDir, "spok-out-*.png")
	if err != nil {
		return nil, "", err
	}
	outPath := outf.Name()
	outf.Close()
	defer os.Remove(outPath)

	cmd := exec.CommandContext(ctx, s.cfg.FFmpegPath,
		"-nostdin",
		"-i", inPath,
		"-lavfi", buildFilter(opts, label),
		"-y", outPath,
	)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return nil, "", ctx.Err()
		}
		s.cfg.Logger.Error("ffmpeg render failed", "err", err, "stderr", stderr.String())
		return nil, "", ErrRender
	}

	png, err = os.ReadFile(outPath)
	if err != nil {
		return nil, "", err
	}
	if s.cache != nil {
		s.cache.put(cacheKey, png, label)
	}
	return png, label, nil
}

func (s *Service) writeTemp(src io.Reader, pattern string) (string, error) {
	f, err := os.CreateTemp(s.cfg.TempDir, pattern)
	if err != nil {
		return "", err
	}
	defer f.Close()
	if _, err := io.Copy(f, src); err != nil {
		os.Remove(f.Name())
		return "", err
	}
	return f.Name(), nil
}

// safeExt returns a short, lower-case, alphanumeric extension (with dot) or ""
// if the client-supplied name isn't one. Keeps arbitrary client input out of
// the temp file name.
func safeExt(ext string) string {
	ext = strings.ToLower(ext)
	if !strings.HasPrefix(ext, ".") || len(ext) > 6 {
		return ""
	}
	for _, c := range ext[1:] {
		if !((c >= 'a' && c <= 'z') || (c >= '0' && c <= '9')) {
			return ""
		}
	}
	return ext
}
