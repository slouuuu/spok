# spok

A lightweight HTTP server (and embeddable Go package) that generates audio spectrograms. Send it an audio file, get back a PNG spectrogram with optional codec, sample rate, and bitrate metadata.

Built with Go and FFmpeg. Supports mp3, ogg, flac, wav, aac, opus, and other formats ffmpeg can demux.

## Requirements

- [Go](https://go.dev/) 1.22+
- [FFmpeg](https://ffmpeg.org/) (both `ffmpeg` and `ffprobe` must be in your `$PATH`)

## Building and running

```bash
git clone https://github.com/slouuuu/spok.git
cd spok
go build -o spok ./cmd/spok
./spok
```

The server listens on `:9090` by default. Flags override environment variables, which override built-in defaults:

| Flag | Env | Default | Meaning |
|------|-----|---------|---------|
| `-addr` | `SPOK_ADDR` | `:9090` | Listen address |
| `-ffmpeg` | `SPOK_FFMPEG` | `ffmpeg` | ffmpeg binary |
| `-ffprobe` | `SPOK_FFPROBE` | `ffprobe` | ffprobe binary |
| `-timeout` | | `30s` | Per-render timeout |
| `-max-concurrent` | | NumCPU | Max simultaneous renders |
| `-max-upload` | | `100MiB` | Max upload size (`-1` unlimited) |
| `-allow-overrides` | | `false` | Allow `?w=&h=&color=...` query overrides |
| `-cache-size` | | `0` | In-memory LRU entries (`0` disabled) |
| `-width` / `-height` | | `1024` / `512` | Default spectrogram size |
| `-color` / `-scale` / `-mode` / `-win` / `-gain` | | `intensity` / `log` / `combined` / `hann` / `1` | Default render options |
| `-legend` / `-overlay` | | `false` / `true` | Axis legend / metadata overlay |

```bash
SPOK_ADDR=:9090 ./spok -cache-size 64 -allow-overrides
```

## API

`POST /analyze` with a multipart `file` field. Successful responses are `image/png`; probed metadata is also returned in the `X-Spok-Audio` header (for example `MP3 44.1 kHz, 320 kbps`).

When `-allow-overrides` is enabled, callers may pass query params: `w`, `h`, `color`, `scale`, `mode`, `win`, `gain`, `legend`, `overlay`. Dimensions are clamped to configured limits.

#### curl

```bash
curl -F file=@song.mp3 http://localhost:9090/analyze -o spectrogram.png
```

#### Python

```python
import requests

with open("song.mp3", "rb") as f:
    res = requests.post("http://localhost:9090/analyze", files={"file": f})

with open("spectrogram.png", "wb") as out:
    out.write(res.content)
print(res.headers.get("X-Spok-Audio"))
```

## Embedding

Import the package and mount `Service.Handler()` on your own mux, or call `Service.Render` directly:

```go
svc, err := spok.New(spok.Config{
    MaxConcurrent: 4,
    RenderTimeout: 20 * time.Second,
    CacheSize:     128, // optional; 0 disables
    Defaults: spok.RenderOptions{
        Color:   "magma",
        Scale:   "log",
        Overlay: true,
    },
})
if err != nil {
    log.Fatal(err)
}
mux.Handle("/spok/", http.StripPrefix("/spok", svc.Handler()))
```

Zero-valued `Config` fields get safe defaults. Request overrides stay off unless `AllowRequestOverrides` is set, so an embedded instance renders exactly what the host configured.

### Caching

Set `Config.CacheSize` (or `-cache-size`) to enable an in-memory LRU. The key is the SHA-256 of the upload bytes combined with the resolved `RenderOptions`, so different render settings never collide. On a hit, spok returns the cached PNG and `X-Spok-Audio` label without running ffmpeg. Caching is disabled by default (`CacheSize == 0`).

## Docker

```bash
docker build -t spok .
docker run -p 9090:9090 spok
```

```bash
docker run -p 8080:8080 -e SPOK_ADDR=:8080 spok
```

## License

GPL 3.0
