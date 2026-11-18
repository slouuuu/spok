package spok

import (
	"bytes"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// tinyPNG is a minimal valid 1x1 PNG used by the ffmpeg stub.
var tinyPNG = []byte{
	0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a, 0x00, 0x00, 0x00, 0x0d,
	0x49, 0x48, 0x44, 0x52, 0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01,
	0x08, 0x02, 0x00, 0x00, 0x00, 0x90, 0x77, 0x53, 0xde, 0x00, 0x00, 0x00,
	0x0c, 0x49, 0x44, 0x41, 0x54, 0x78, 0x9c, 0x63, 0xf8, 0xcf, 0xc0, 0x00,
	0x00, 0x03, 0x01, 0x01, 0x00, 0xc9, 0xfe, 0x92, 0xef, 0x00, 0x00, 0x00,
	0x00, 0x49, 0x45, 0x4e, 0x44, 0xae, 0x42, 0x60, 0x82,
}

// writeStubBins creates tiny shell helpers that stand in for ffmpeg/ffprobe.
// ffmpeg copies a canned PNG to its last argument and appends argv to a log file.
// ffprobe emits a minimal JSON probe result on stdout.
func writeStubBins(t *testing.T) (ffmpeg, ffprobe, ffmpegLog string) {
	t.Helper()
	dir := t.TempDir()
	ffmpegLog = filepath.Join(dir, "ffmpeg.log")
	pngPath := filepath.Join(dir, "stub.png")
	if err := os.WriteFile(pngPath, tinyPNG, 0o644); err != nil {
		t.Fatal(err)
	}

	ffmpeg = filepath.Join(dir, "ffmpeg")
	ffprobe = filepath.Join(dir, "ffprobe")

	ffmpegScript := "#!/bin/sh\n" +
		"log=" + shellQuote(ffmpegLog) + "\n" +
		"png=" + shellQuote(pngPath) + "\n" +
		"printf '%s\\n' \"$@\" >> \"$log\"\n" +
		"out=\n" +
		"for a in \"$@\"; do out=$a; done\n" +
		"cp \"$png\" \"$out\"\n"
	if err := os.WriteFile(ffmpeg, []byte(ffmpegScript), 0o755); err != nil {
		t.Fatal(err)
	}

	ffprobeScript := `#!/bin/sh
cat <<'EOF'
{"streams":[{"codec_name":"mp3","sample_rate":"44100","bit_rate":"320000"}],"format":{"bit_rate":"320000"}}
EOF
`
	if err := os.WriteFile(ffprobe, []byte(ffprobeScript), 0o755); err != nil {
		t.Fatal(err)
	}
	return ffmpeg, ffprobe, ffmpegLog
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

func TestHandlerMissingFile(t *testing.T) {
	ffmpeg, ffprobe, _ := writeStubBins(t)
	svc, err := New(Config{
		FFmpegPath:  ffmpeg,
		FFprobePath: ffprobe,
		Defaults:    RenderOptions{Overlay: false},
	})
	if err != nil {
		t.Fatal(err)
	}

	ts := httptest.NewServer(svc.Handler())
	t.Cleanup(ts.Close)

	res, err := http.Post(ts.URL+"/analyze", "application/x-www-form-urlencoded", strings.NewReader(""))
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusBadRequest {
		body, _ := io.ReadAll(res.Body)
		t.Fatalf("status = %d, want 400; body %s", res.StatusCode, body)
	}
}

func TestHandlerIgnoresOverridesWhenDisabled(t *testing.T) {
	ffmpeg, ffprobe, logPath := writeStubBins(t)
	svc, err := New(Config{
		FFmpegPath:            ffmpeg,
		FFprobePath:           ffprobe,
		AllowRequestOverrides: false,
		Defaults: RenderOptions{
			Color:   "intensity",
			Overlay: false,
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	ts := httptest.NewServer(svc.Handler())
	t.Cleanup(ts.Close)

	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, err := mw.CreateFormFile("file", "tone.wav")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fw.Write([]byte("not-really-audio")); err != nil {
		t.Fatal(err)
	}
	mw.Close()

	req, err := http.NewRequest(http.MethodPost, ts.URL+"/analyze?color=magma&w=200", &buf)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(res.Body)
		t.Fatalf("status = %d, want 200; body %s", res.StatusCode, body)
	}

	logBytes, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	log := string(logBytes)
	if !strings.Contains(log, "color=intensity") {
		t.Fatalf("expected default color=intensity in ffmpeg argv, got:\n%s", log)
	}
	if strings.Contains(log, "color=magma") {
		t.Fatalf("query override leaked into ffmpeg argv:\n%s", log)
	}
	if strings.Contains(log, "s=200x") {
		t.Fatalf("width override leaked into ffmpeg argv:\n%s", log)
	}

	r := httptest.NewRequest(http.MethodPost, "/analyze?color=magma&w=200", nil)
	opts, err := svc.renderOptionsFromRequest(r)
	if err != nil {
		t.Fatal(err)
	}
	if opts.Color != "intensity" || opts.Width != 1024 {
		t.Fatalf("overrides applied when disabled: %+v", opts)
	}
}

func TestHandlerRealFFmpeg(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping real ffmpeg test in short mode")
	}
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not installed")
	}
	if _, err := exec.LookPath("ffprobe"); err != nil {
		t.Skip("ffprobe not installed")
	}

	svc, err := New(Config{Defaults: RenderOptions{Overlay: true}})
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(svc.Handler())
	t.Cleanup(ts.Close)

	wav := synthesizeWAV(t)

	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, err := mw.CreateFormFile("file", "tone.wav")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.Copy(fw, bytes.NewReader(wav)); err != nil {
		t.Fatal(err)
	}
	mw.Close()

	res, err := http.Post(ts.URL+"/analyze", mw.FormDataContentType(), &buf)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(res.Body)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, body %s", res.StatusCode, body)
	}
	if len(body) < 8 || string(body[:8]) != "\x89PNG\r\n\x1a\n" {
		t.Fatalf("response is not a PNG (%d bytes)", len(body))
	}
	if ct := res.Header.Get("Content-Type"); ct != "image/png" {
		t.Fatalf("Content-Type = %q", ct)
	}
}

// synthesizeWAV builds a tiny mono PCM WAV via ffmpeg so the real integration
// test has a valid audio upload without shipping a fixture.
func synthesizeWAV(t *testing.T) []byte {
	t.Helper()
	out := filepath.Join(t.TempDir(), "tone.wav")
	cmd := exec.Command("ffmpeg", "-f", "lavfi", "-i", "sine=frequency=440:duration=0.1",
		"-ac", "1", "-ar", "8000", "-y", out)
	if outBytes, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("ffmpeg synthesize: %v\n%s", err, outBytes)
	}
	b, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	return b
}
