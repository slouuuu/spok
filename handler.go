package spok

import (
	"context"
	"errors"
	"net/http"
	"path/filepath"
	"strconv"
)

// Handler returns an http.Handler exposing POST /analyze. Mount it wherever you
// like in your own mux, e.g. mux.Handle("/spok/", http.StripPrefix("/spok",
// svc.Handler())).
func (s *Service) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /analyze", s.handleAnalyze)
	return mux
}

func (s *Service) handleAnalyze(w http.ResponseWriter, r *http.Request) {
	if s.cfg.MaxUploadBytes >= 0 {
		r.Body = http.MaxBytesReader(w, r.Body, s.cfg.MaxUploadBytes)
	}

	file, hdr, err := r.FormFile("file")
	if err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			http.Error(w, "upload too large", http.StatusRequestEntityTooLarge)
			return
		}
		http.Error(w, "missing 'file' field", http.StatusBadRequest)
		return
	}
	defer file.Close()

	opts, err := s.renderOptionsFromRequest(r)
	if err != nil {
		http.Error(w, "invalid render options: "+err.Error(), http.StatusBadRequest)
		return
	}

	png, label, err := s.Render(r.Context(), file, filepath.Ext(hdr.Filename), opts)
	if err != nil {
		switch {
		case errors.Is(err, context.Canceled):
			// Client disconnected; nothing useful to send.
		case errors.Is(err, context.DeadlineExceeded):
			http.Error(w, "render timed out", http.StatusGatewayTimeout)
		default:
			http.Error(w, "could not render spectrogram", http.StatusUnprocessableEntity)
		}
		return
	}

	if label != "" {
		w.Header().Set("X-Spok-Audio", label) // metadata for scripts, not just baked on the image
	}
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Content-Length", strconv.Itoa(len(png)))
	w.Write(png)
}

// renderOptionsFromRequest starts from the configured defaults and, when
// overrides are allowed, applies recognized query params, clamps dimensions to
// the configured limits, and validates the result.
func (s *Service) renderOptionsFromRequest(r *http.Request) (RenderOptions, error) {
	opts := s.cfg.Defaults
	if !s.cfg.AllowRequestOverrides {
		return opts, nil
	}

	q := r.URL.Query()
	if v := q.Get("w"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			return opts, errors.New("w must be an integer")
		}
		opts.Width = n
	}
	if v := q.Get("h"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			return opts, errors.New("h must be an integer")
		}
		opts.Height = n
	}
	if v := q.Get("gain"); v != "" {
		g, err := strconv.ParseFloat(v, 64)
		if err != nil {
			return opts, errors.New("gain must be a number")
		}
		opts.Gain = g
	}
	if v := q.Get("color"); v != "" {
		opts.Color = ColorMode(v)
	}
	if v := q.Get("scale"); v != "" {
		opts.Scale = ScaleMode(v)
	}
	if v := q.Get("mode"); v != "" {
		opts.Channel = ChannelMode(v)
	}
	if v := q.Get("win"); v != "" {
		opts.WinFunc = WinFunc(v)
	}
	if v := q.Get("legend"); v != "" {
		opts.Legend = v == "1" || v == "true"
	}
	if v := q.Get("overlay"); v != "" {
		opts.Overlay = v == "1" || v == "true"
	}

	opts = s.cfg.Limits.clamp(opts)
	if err := opts.validate(); err != nil {
		return opts, err
	}
	return opts, nil
}
