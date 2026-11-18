package spok

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

// probeResult maps the subset of ffprobe's JSON output we use. JSON is parsed by
// key, so a missing or reordered field never silently shifts another's value
// (unlike positional CSV).
type probeResult struct {
	Streams []struct {
		CodecName  string `json:"codec_name"`
		SampleRate string `json:"sample_rate"`
		BitRate    string `json:"bit_rate"`
	} `json:"streams"`
	Format struct {
		BitRate string `json:"bit_rate"`
	} `json:"format"`
}

// probe returns a human-readable label like "MP3 44.1 kHz, 320 kbps", or "" if
// ffprobe can't determine it. Failures are logged, never fatal.
func (s *Service) probe(ctx context.Context, path string) string {
	out, err := exec.CommandContext(ctx, s.cfg.FFprobePath,
		"-v", "quiet",
		"-select_streams", "a:0",
		"-show_entries", "stream=codec_name,sample_rate,bit_rate:format=bit_rate",
		"-of", "json",
		path,
	).Output()
	if err != nil {
		s.cfg.Logger.Warn("ffprobe failed", "err", err)
		return ""
	}

	var pr probeResult
	if err := json.Unmarshal(out, &pr); err != nil || len(pr.Streams) == 0 {
		return ""
	}
	st := pr.Streams[0]

	label := strings.ToUpper(st.CodecName)
	if sr, err := strconv.Atoi(st.SampleRate); err == nil {
		label += fmt.Sprintf(" %.1f kHz", float64(sr)/1000)
	}

	// Stream bitrate is absent for some codecs (flac, vorbis); fall back to the
	// container-level figure.
	br := st.BitRate
	if br == "" || br == "N/A" {
		br = pr.Format.BitRate
	}
	if n, err := strconv.Atoi(br); err == nil {
		label += fmt.Sprintf(", %d kbps", n/1000)
	}
	return label
}
