package main

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

func main() {
	// you can set SPOK_ADDR if 9090 is taken or whatever
	addr := os.Getenv("SPOK_ADDR")
	if addr == "" {
		addr = ":9090"
	}

	// only one route lol, keeps things simple
	http.HandleFunc("POST /analyze", handleAnalyze)

	log.Printf("spok listening on %s", addr)
	log.Fatal(http.ListenAndServe(addr, nil))
}

// handleAnalyze takes an audio file upload, runs it through ffmpeg,
// and sends back a spectrogram png. thats literaly it
func handleAnalyze(w http.ResponseWriter, r *http.Request) {
	// grab the uploaded file from the form
	file, hdr, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "missing file field", http.StatusBadRequest)
		return
	}
	defer file.Close()

	// save it to a temp file so ffmpeg can read it
	// (ffmpeg needs an actual file path, cant just pipe stdin for this filter)
	tmp, err := os.CreateTemp("", "spok-*"+filepath.Ext(hdr.Filename))
	if err != nil {
		http.Error(w, "failed to create temp file", http.StatusInternalServerError)
		return
	}
	defer os.Remove(tmp.Name())
	defer tmp.Close()

	if _, err := io.Copy(tmp, file); err != nil {
		http.Error(w, "failed to write temp file", http.StatusInternalServerError)
		return
	}
	tmp.Close() // close before ffmpeg reads it

	// ask ffprobe what codec and bitrate we're dealing with
	bitrateLabel := probeBitrate(tmp.Name())

	// another temp file for the output png
	out, err := os.CreateTemp("", "spok-out-*.png")
	if err != nil {
		http.Error(w, "failed to create output file", http.StatusInternalServerError)
		return
	}
	defer os.Remove(out.Name())
	out.Close()

	// the ffmpeg filter chain:
	// 1. showspectrumpic does the actual spectrogram rendering (1024x512, log scale)
	// 2. drawtext slaps the codec/bitrate info on top so you can see what you're looking at
	filter := "showspectrumpic=s=1024x512:mode=combined:color=intensity:scale=log"
	if bitrateLabel != "" {
		// colons need escaping or ffmpeg gets confused
		escaped := strings.ReplaceAll(bitrateLabel, ":", "\\:")
		filter += fmt.Sprintf(",drawtext=text='%s':x=10:y=10:fontsize=18:fontcolor=white:borderw=2:bordercolor=black", escaped)
	}

	// let ffmpeg do its thing
	cmd := exec.Command("ffmpeg",
		"-i", tmp.Name(),
		"-lavfi", filter,
		"-y", out.Name(),
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		log.Printf("ffmpeg error: %s\n%s", err, output)
		http.Error(w, fmt.Sprintf("ffmpeg failed: %s", err), http.StatusInternalServerError)
		return
	}

	// send back the pretty picture
	w.Header().Set("Content-Type", "image/png")
	http.ServeFile(w, r, out.Name())
}

// probeBitrate uses ffprobe to figure out what codec, sample rate
// and bitrate the audio file has. returns something like "MP3 44.1 kHz, 320 kbps"
// or an empty string if ffprobe cant figure it out (not the end of the world)
func probeBitrate(path string) string {
	// first try: get stream-level info
	brOut, err := exec.Command("ffprobe",
		"-v", "quiet",
		"-select_streams", "a:0",
		"-show_entries", "stream=bit_rate,codec_name,sample_rate",
		"-of", "csv=p=0",
		path,
	).Output()
	if err != nil {
		return ""
	}

	// ffprobe gives us csv like: mp3,44100,320000
	parts := strings.Split(strings.TrimSpace(string(brOut)), ",")
	if len(parts) < 3 {
		return ""
	}

	codec := strings.ToUpper(parts[0])
	sampleRate := parts[1]
	bitrate := parts[2]

	var label string

	// make sample rate human readable (44100 -> 44.1 kHz)
	if sr, err := strconv.Atoi(sampleRate); err == nil {
		label = fmt.Sprintf("%s %.1f kHz", codec, float64(sr)/1000)
	} else {
		label = codec
	}

	// try to get the bitrate in kbps
	if br, err := strconv.Atoi(bitrate); err == nil {
		label += fmt.Sprintf(", %d kbps", br/1000)
	} else if bitrate != "N/A" && bitrate != "" {
		// some formats (like flac, vorbis) dont report stream-level bitrate
		// so we fall back to the container-level bitrate instead.
		// its not perfect but better than nothing
		cOut, err := exec.Command("ffprobe",
			"-v", "quiet",
			"-show_entries", "format=bit_rate",
			"-of", "csv=p=0",
			path,
		).Output()
		if err == nil {
			if cbr, err := strconv.Atoi(strings.TrimSpace(string(cOut))); err == nil {
				label += fmt.Sprintf(", %d kbps", cbr/1000)
			}
		}
	}

	return label
}
