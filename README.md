# spok

A lightweight HTTP server that generates audio spectrograms. Send it an audio file, get back a PNG spectrogram with codec, sample rate, and bitrate info overlaid. 

Built with Go and FFmpeg 
Supports : mp3, ogg, flac, wav, aac, opus, etc.

## Requirements

- [Go](https://go.dev/) 1.22+
- [FFmpeg](https://ffmpeg.org/) (both `ffmpeg` and `ffprobe` must be in your `$PATH`)


## Building and running

```bash
git clone https://github.com/slou/spok.git
cd spok
go build -o spok .
./spok
```

The server starts on port `9090` by default. You can change the address with the `SPOK_ADDR` environment variable:

```bash
SPOK_ADDR=:9090 ./spok
```

## API

#### Example with curl

```bash
curl -F file=@song.mp3 http://localhost:9090/analyze -o spectrogram.png
```

#### Example with Python

```python
import requests

with open("song.mp3", "rb") as f:
    res = requests.post("http://localhost:9090/analyze", files={"file": f})

with open("spectrogram.png", "wb") as out:
    out.write(res.content)
```
## Docker

If you don't want to install FFmpeg locally, use Docker:

```bash
docker build -t spok .
docker run -p 9090:9090 spok
```

To change the port:

```bash
docker run -p 8080:8080 -e SPOK_ADDR=:8080 spok
```

## License

MIT
