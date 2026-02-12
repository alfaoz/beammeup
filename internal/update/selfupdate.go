package update

import (
	"archive/tar"
	"bufio"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/alfaoz/beammeup/internal/version"
)

type Result struct {
	Version string
	Updated bool
}

const (
	maxUpdateArchiveBytes    = int64(200 << 20) // 200 MiB
	maxUpdateSHA256SUMSBytes = int64(1 << 20)   // 1 MiB
	maxUpdateBinaryBytes     = int64(80 << 20)  // 80 MiB
)

type ghRelease struct {
	TagName string `json:"tag_name"`
	Assets  []struct {
		Name string `json:"name"`
		URL  string `json:"browser_download_url"`
	} `json:"assets"`
}

func SelfUpdate(baseURL string) (Result, error) {
	execPath, err := os.Executable()
	if err != nil {
		return Result{}, fmt.Errorf("resolve executable path: %w", err)
	}

	osName, archName, err := platformAssetParts()
	if err != nil {
		return Result{}, err
	}
	assetName := fmt.Sprintf("beammeup_%s_%s.tar.gz", osName, archName)

	base := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if base != "" {
		if err := validateBaseURL(base); err != nil {
			return Result{}, err
		}
		res, err := selfUpdateFromMirror(execPath, base, assetName)
		if err == nil {
			return res, nil
		}
		// If the mirror looks compromised (checksum mismatch), don't fall back.
		var ie *integrityError
		if errors.As(err, &ie) {
			return Result{}, err
		}
		// beammeup.pw is the default; fall back to GitHub if the mirror isn't available.
		if base != "https://beammeup.pw" {
			return Result{}, err
		}
	}

	return selfUpdateFromGitHub(execPath, assetName)
}

func platformAssetParts() (string, string, error) {
	var osName string
	switch runtime.GOOS {
	case "darwin":
		osName = "darwin"
	case "linux":
		osName = "linux"
	default:
		return "", "", fmt.Errorf("unsupported OS for self-update: %s", runtime.GOOS)
	}

	var archName string
	switch runtime.GOARCH {
	case "arm64":
		archName = "arm64"
	case "amd64":
		archName = "amd64"
	default:
		return "", "", fmt.Errorf("unsupported arch for self-update: %s", runtime.GOARCH)
	}
	return osName, archName, nil
}

func selfUpdateFromMirror(execPath, base, assetName string) (Result, error) {
	downloadURL := fmt.Sprintf("%s/releases/latest/%s", base, assetName)
	sumsURL := fmt.Sprintf("%s/releases/latest/SHA256SUMS", base)
	versionURL := fmt.Sprintf("%s/releases/latest/version.txt", base)

	newVersionRaw, err := fetchText(versionURL, 1024)
	if err != nil {
		return Result{}, fmt.Errorf("mirror version.txt fetch failed: %w", err)
	}
	newVersion := normalizeVersion(newVersionRaw)
	if newVersion == "" {
		return Result{}, fmt.Errorf("mirror version.txt was empty")
	}
	if newVersion == version.AppVersion {
		return Result{Version: newVersion, Updated: false}, nil
	}

	if err := updateFromURLs(execPath, downloadURL, sumsURL, assetName); err != nil {
		return Result{}, err
	}
	return Result{Version: newVersion, Updated: true}, nil
}

func selfUpdateFromGitHub(execPath, assetName string) (Result, error) {
	downloadURL := fmt.Sprintf("https://github.com/%s/releases/latest/download/%s", version.DefaultRepo, assetName)
	sumsURL := fmt.Sprintf("https://github.com/%s/releases/latest/download/SHA256SUMS", version.DefaultRepo)
	versionURL := fmt.Sprintf("https://github.com/%s/releases/latest/download/version.txt", version.DefaultRepo)

	newVersion := ""
	if v, err := fetchText(versionURL, 1024); err == nil {
		newVersion = normalizeVersion(v)
	}
	if newVersion == "" {
		rel, err := fetchLatestRelease()
		if err != nil {
			return Result{}, err
		}
		newVersion = strings.TrimPrefix(strings.TrimSpace(rel.TagName), "v")
	}
	if newVersion == "" {
		return Result{}, errors.New("could not determine latest release version")
	}
	if newVersion == version.AppVersion {
		return Result{Version: newVersion, Updated: false}, nil
	}

	if err := updateFromURLs(execPath, downloadURL, sumsURL, assetName); err != nil {
		return Result{}, err
	}
	return Result{Version: newVersion, Updated: true}, nil
}

func normalizeVersion(v string) string {
	v = strings.TrimSpace(v)
	v = strings.TrimPrefix(v, "v")
	return v
}

func fetchLatestRelease() (ghRelease, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/releases/latest", version.DefaultRepo)
	resp, err := (&http.Client{Timeout: 20 * time.Second}).Get(url)
	if err != nil {
		return ghRelease{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return ghRelease{}, fmt.Errorf("github release lookup failed: %s %s", resp.Status, strings.TrimSpace(string(b)))
	}
	var rel ghRelease
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		return ghRelease{}, err
	}
	if rel.TagName == "" {
		return ghRelease{}, errors.New("release tag_name missing")
	}
	return rel, nil
}

type integrityError struct {
	msg string
}

func (e *integrityError) Error() string { return e.msg }

func updateFromURLs(execPath, downloadURL, sumsURL, assetName string) error {
	tmpDir, err := os.MkdirTemp("", "beammeup-update-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmpDir)

	archivePath := filepath.Join(tmpDir, assetName)
	if err := downloadTo(downloadURL, archivePath, maxUpdateArchiveBytes); err != nil {
		return err
	}

	sums, err := fetchText(sumsURL, maxUpdateSHA256SUMSBytes)
	if err != nil {
		return fmt.Errorf("failed to download SHA256SUMS: %w", err)
	}
	if err := verifyChecksum(sums, assetName, archivePath); err != nil {
		return err
	}

	binPath := filepath.Join(tmpDir, "beammeup-new")
	if err := extractBinary(archivePath, binPath, maxUpdateBinaryBytes); err != nil {
		return err
	}

	if err := os.Chmod(binPath, 0o755); err != nil {
		return err
	}

	backup := execPath + ".bak"
	_ = os.Remove(backup)
	if err := os.Rename(execPath, backup); err != nil {
		return fmt.Errorf("prepare executable replacement: %w", err)
	}
	if err := os.Rename(binPath, execPath); err != nil {
		_ = os.Rename(backup, execPath)
		return fmt.Errorf("replace executable: %w", err)
	}
	_ = os.Remove(backup)

	return nil
}

func verifyChecksum(sumsText, assetName, archivePath string) error {
	want, err := expectedSHA256(sumsText, assetName)
	if err != nil {
		return err
	}
	got, err := sha256File(archivePath)
	if err != nil {
		return err
	}
	if !strings.EqualFold(want, got) {
		return &integrityError{msg: fmt.Sprintf("checksum mismatch for %s: want %s got %s", assetName, want, got)}
	}
	return nil
}

func expectedSHA256(sumsText, assetName string) (string, error) {
	s := bufio.NewScanner(strings.NewReader(sumsText))
	for s.Scan() {
		line := strings.TrimSpace(s.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		hash := fields[0]
		name := strings.TrimPrefix(fields[1], "*")
		name = strings.TrimPrefix(name, "./")
		if filepath.Base(name) == assetName || filepath.Base(strings.TrimPrefix(name, "dist/")) == assetName {
			return hash, nil
		}
	}
	if err := s.Err(); err != nil {
		return "", err
	}
	return "", fmt.Errorf("SHA256SUMS missing entry for %q", assetName)
}

func sha256File(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func downloadTo(url, path string, maxBytes int64) error {
	resp, err := (&http.Client{Timeout: 120 * time.Second}).Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return fmt.Errorf("download failed: %s %s", resp.Status, strings.TrimSpace(string(b)))
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	var r io.Reader = resp.Body
	if maxBytes > 0 {
		r = io.LimitReader(resp.Body, maxBytes+1)
	}
	n, err := io.Copy(f, r)
	if err != nil {
		return err
	}
	if maxBytes > 0 && n > maxBytes {
		return fmt.Errorf("download exceeded max size (%d bytes)", maxBytes)
	}
	return nil
}

func extractBinary(archivePath, dst string, maxBytes int64) error {
	f, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer f.Close()
	gr, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer gr.Close()
	tr := tar.NewReader(gr)

	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		name := filepath.Base(h.Name)
		if name != "beammeup" {
			continue
		}
		if h.Typeflag != tar.TypeReg && h.Typeflag != tar.TypeRegA {
			return fmt.Errorf("unexpected tar entry type for beammeup: %v", h.Typeflag)
		}
		if h.Size <= 0 {
			return errors.New("invalid binary size in archive")
		}
		if maxBytes > 0 && h.Size > maxBytes {
			return fmt.Errorf("refusing to extract beammeup binary larger than %d bytes", maxBytes)
		}

		out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o700)
		if err != nil {
			return err
		}
		if _, err := io.CopyN(out, tr, h.Size); err != nil {
			_ = out.Close()
			return err
		}
		if err := out.Close(); err != nil {
			return err
		}
		return nil
	}
	return errors.New("binary not found in archive")
}

func fetchText(url string, maxBytes int64) (string, error) {
	resp, err := (&http.Client{Timeout: 10 * time.Second}).Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return "", fmt.Errorf("status: %s", resp.Status)
	}
	limit := maxBytes
	if limit <= 0 {
		limit = 1024
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, limit))
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func validateBaseURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("invalid --base-url: %w", err)
	}
	switch strings.ToLower(u.Scheme) {
	case "https":
		return nil
	case "http":
		host := strings.ToLower(u.Hostname())
		if host == "localhost" || host == "127.0.0.1" || host == "::1" {
			return nil
		}
		return fmt.Errorf("refusing insecure base URL %q (http). Use https, or localhost for dev", raw)
	default:
		return fmt.Errorf("invalid --base-url scheme %q (use https)", u.Scheme)
	}
}
