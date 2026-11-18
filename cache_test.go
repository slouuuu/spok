package spok

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestLRUHitMissEviction(t *testing.T) {
	c := newLRU(2)
	if _, _, ok := c.get("missing"); ok {
		t.Fatal("expected miss")
	}

	c.put("a", []byte("png-a"), "label-a")
	c.put("b", []byte("png-b"), "label-b")
	if c.len() != 2 {
		t.Fatalf("len = %d, want 2", c.len())
	}

	png, label, ok := c.get("a")
	if !ok || string(png) != "png-a" || label != "label-a" {
		t.Fatalf("get(a) = %q %q %v", png, label, ok)
	}

	// Inserting a third entry should evict the least-recently used ("b"),
	// since "a" was just touched.
	c.put("c", []byte("png-c"), "label-c")
	if c.len() != 2 {
		t.Fatalf("len after eviction = %d, want 2", c.len())
	}
	if _, _, ok := c.get("b"); ok {
		t.Fatal("b should have been evicted")
	}
	if _, _, ok := c.get("a"); !ok {
		t.Fatal("a should still be cached")
	}
	if _, _, ok := c.get("c"); !ok {
		t.Fatal("c should be cached")
	}

	// Updating an existing key refreshes value without growing.
	c.put("a", []byte("png-a2"), "label-a2")
	if c.len() != 2 {
		t.Fatalf("len after update = %d, want 2", c.len())
	}
	png, label, ok = c.get("a")
	if !ok || string(png) != "png-a2" || label != "label-a2" {
		t.Fatalf("updated get(a) = %q %q %v", png, label, ok)
	}
}

func TestLRUNilDisabled(t *testing.T) {
	var c *lruCache
	c.put("x", []byte("y"), "z")
	if _, _, ok := c.get("x"); ok {
		t.Fatal("nil cache should miss")
	}
	if newLRU(0) != nil {
		t.Fatal("newLRU(0) should return nil")
	}
}

func TestMakeCacheKeyDistinguishesOptions(t *testing.T) {
	sum := []byte("content-hash")
	a := RenderOptions{}.withDefaults()
	b := a
	b.Color = "magma"
	if makeCacheKey(sum, a) == makeCacheKey(sum, b) {
		t.Fatal("different options produced the same cache key")
	}
	if makeCacheKey(sum, a) != makeCacheKey(sum, a) {
		t.Fatal("identical inputs should produce a stable key")
	}
	if makeCacheKey([]byte("other"), a) == makeCacheKey(sum, a) {
		t.Fatal("different content produced the same cache key")
	}
}

func TestRenderUsesCache(t *testing.T) {
	dir := t.TempDir()
	counterPath := filepath.Join(dir, "count")
	pngPath := filepath.Join(dir, "stub.png")
	if err := os.WriteFile(pngPath, tinyPNG, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(counterPath, []byte("0"), 0o644); err != nil {
		t.Fatal(err)
	}

	ffmpeg := filepath.Join(dir, "ffmpeg")
	ffprobe := filepath.Join(dir, "ffprobe")
	ffmpegScript := "#!/bin/sh\n" +
		"c=" + shellQuote(counterPath) + "\n" +
		"n=$(cat \"$c\")\n" +
		"echo $((n+1)) > \"$c\"\n" +
		"png=" + shellQuote(pngPath) + "\n" +
		"out=\n" +
		"for a in \"$@\"; do out=$a; done\n" +
		"cp \"$png\" \"$out\"\n"
	if err := os.WriteFile(ffmpeg, []byte(ffmpegScript), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(ffprobe, []byte("#!/bin/sh\necho '{}'\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	svc, err := New(Config{
		FFmpegPath:  ffmpeg,
		FFprobePath: ffprobe,
		CacheSize:   4,
		Defaults:    RenderOptions{Overlay: false},
	})
	if err != nil {
		t.Fatal(err)
	}

	audio := []byte("identical-audio-bytes")
	opts := RenderOptions{Overlay: false}.withDefaults()

	png1, _, err := svc.Render(context.Background(), bytes.NewReader(audio), ".wav", opts)
	if err != nil {
		t.Fatal(err)
	}
	png2, _, err := svc.Render(context.Background(), bytes.NewReader(audio), ".wav", opts)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(png1, png2) {
		t.Fatal("cached render returned different PNG")
	}

	countRaw, err := os.ReadFile(counterPath)
	if err != nil {
		t.Fatal(err)
	}
	n, err := strconv.Atoi(strings.TrimSpace(string(countRaw)))
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("ffmpeg invocations = %d, want 1 (second call should be a cache hit)", n)
	}

	// Different options must miss the cache and invoke ffmpeg again.
	opts2 := opts
	opts2.Color = "magma"
	if _, _, err := svc.Render(context.Background(), bytes.NewReader(audio), ".wav", opts2); err != nil {
		t.Fatal(err)
	}
	countRaw, _ = os.ReadFile(counterPath)
	n, _ = strconv.Atoi(strings.TrimSpace(string(countRaw)))
	if n != 2 {
		t.Fatalf("ffmpeg invocations after options change = %d, want 2", n)
	}
}
