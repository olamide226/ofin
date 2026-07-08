// Package download fetches the Òfin chat model on first launch.
package download

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

const (
	// ModelURL is the Hugging Face download URL for the baked Òfin GGUF.
	ModelURL = "https://huggingface.co/olamide226/ofin-model/resolve/main/ofin-model.gguf"

	// MinBytes is the sanity floor — a 3B Q4_K_M GGUF is ~1.88 GB.
	MinBytes = 1_500_000_000
)

// Progress is called periodically during download with bytes downloaded
// and total bytes (total is 0 if Content-Length wasn't sent).
type Progress func(downloaded, total int64)

// Model downloads the Òfin GGUF to destPath with resume support.
// Returns the number of bytes downloaded (0 if already complete).
func Model(destPath string, onProgress Progress) (int64, error) {
	// Idempotent: skip if already downloaded and complete.
	if info, err := os.Stat(destPath); err == nil && info.Size() >= MinBytes {
		return 0, nil
	}

	// Ensure parent directory exists.
	if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
		return 0, fmt.Errorf("creating model directory: %w", err)
	}

	// Check for partial file for resume.
	var existingBytes int64
	if info, err := os.Stat(destPath); err == nil {
		existingBytes = info.Size()
	}

	tmpPath := destPath + ".tmp"

	// If we have a partial download, resume from there.
	// Otherwise, start fresh to a temp file.
	var offset int64
	if existingBytes > 0 && existingBytes < MinBytes {
		// Resume: copy partial to tmp and continue.
		if err := os.Rename(destPath, tmpPath); err != nil {
			// Can't move — start fresh.
			existingBytes = 0
		} else {
			offset = existingBytes
		}
	}

	client := &http.Client{Timeout: 30 * time.Minute}
	req, err := http.NewRequest("GET", ModelURL, nil)
	if err != nil {
		return 0, fmt.Errorf("creating request: %w", err)
	}
	if offset > 0 {
		req.Header.Set("Range", fmt.Sprintf("bytes=%d-", offset))
	}

	resp, err := client.Do(req)
	if err != nil {
		return 0, fmt.Errorf("downloading model: %w", err)
	}
	defer resp.Body.Close()

	// Handle resume response: 206 Partial Content, or 200 if server ignores Range.
	var total int64
	switch resp.StatusCode {
	case http.StatusOK:
		// Full download — reset.
		offset = 0
		os.Remove(tmpPath)
		total = resp.ContentLength
	case http.StatusPartialContent:
		total = offset + resp.ContentLength
	default:
		return 0, fmt.Errorf("unexpected HTTP %d from model server", resp.StatusCode)
	}

	// Open temp file for writing (append if resuming).
	flag := os.O_CREATE | os.O_WRONLY
	if offset > 0 {
		flag |= os.O_APPEND
	} else {
		flag |= os.O_TRUNC
	}
	f, err := os.OpenFile(tmpPath, flag, 0644)
	if err != nil {
		return 0, fmt.Errorf("creating temp file: %w", err)
	}
	defer f.Close()

	// Copy with progress.
	buf := make([]byte, 256*1024) // 256 KB buffer
	var downloaded int64
	for {
		n, readErr := resp.Body.Read(buf)
		if n > 0 {
			if _, writeErr := f.Write(buf[:n]); writeErr != nil {
				return 0, fmt.Errorf("writing temp file: %w", writeErr)
			}
			downloaded += int64(n)
			if onProgress != nil {
				onProgress(offset+downloaded, total)
			}
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return 0, fmt.Errorf("reading response: %w", readErr)
		}
	}

	// Verify size.
	totalBytes := offset + downloaded
	if totalBytes < MinBytes {
		return 0, fmt.Errorf("downloaded file too small (%d bytes, expected ≥%d) — network interrupted? delete %s and retry",
			totalBytes, MinBytes, destPath)
	}

	// Atomic rename.
	if err := os.Rename(tmpPath, destPath); err != nil {
		return 0, fmt.Errorf("installing model: %w", err)
	}

	return downloaded, nil
}
