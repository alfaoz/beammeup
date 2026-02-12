package update

import (
	"archive/tar"
	"compress/gzip"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
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

	downloadURL := ""
	newVersion := ""
	base := strings.TrimRight(strings.TrimSpace(baseURL), "/")

	if base != "" && base != "https://beammeup.pw" {
		downloadURL = fmt.Sprintf("%s/releases/latest/%s", base, assetName)
		if v, err := fetchText(fmt.Sprintf("%s/releases/latest/version.txt", base)); err == nil {
			newVersion = strings.TrimSpace(v)
		}
	} else {
		release, err := fetchLatestRelease()
		if err != nil {
			return Result{}, err
		}
		newVersion = strings.TrimPrefix(strings.TrimSpace(release.TagName), "v")
		for _, a := range release.Assets {
			if a.Name == assetName {
				downloadURL = a.URL
				break
			}
		}
	}

	if downloadURL == "" {
		return Result{}, fmt.Errorf("could not resolve release asset %q", assetName)
	}

	tmpDir, err := os.MkdirTemp("", "beammeup-update-*")
	if err != nil {
		return Result{}, err
	}
	defer os.RemoveAll(tmpDir)

	archivePath := filepath.Join(tmpDir, assetName)
	if err := downloadTo(downloadURL, archivePath); err != nil {
		return Result{}, err
	}

	binPath := filepath.Join(tmpDir, "beammeup-new")
	if err := extractBinary(archivePath, binPath); err != nil {
		return Result{}, err
	}

	if err := os.Chmod(binPath, 0o755); err != nil {
		return Result{}, err
	}

	if newVersion == "" {
		newVersion = version.AppVersion
	}

	if strings.TrimPrefix(newVersion, "v") == version.AppVersion {
		return Result{Version: newVersion, Updated: false}, nil
	}

	backup := execPath + ".bak"
	_ = os.Remove(backup)
	if err := os.Rename(execPath, backup); err != nil {
		return Result{}, fmt.Errorf("prepare executable replacement: %w", err)
	}
	if err := os.Rename(binPath, execPath); err != nil {
		_ = os.Rename(backup, execPath)
		return Result{}, fmt.Errorf("replace executable: %w", err)
	}
	_ = os.Remove(backup)

	return Result{Version: newVersion, Updated: true}, nil
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

func downloadTo(url, path string) error {
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
	_, err = io.Copy(f, resp.Body)
	return err
}

func extractBinary(archivePath, dst string) error {
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
		out, err := os.Create(dst)
		if err != nil {
			return err
		}
		if _, err := io.Copy(out, tr); err != nil {
			out.Close()
			return err
		}
		if err := out.Close(); err != nil {
			return err
		}
		return nil
	}
	return errors.New("binary not found in archive")
}

func fetchText(url string) (string, error) {
	resp, err := (&http.Client{Timeout: 10 * time.Second}).Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return "", fmt.Errorf("status: %s", resp.Status)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, 1024))
	if err != nil {
		return "", err
	}
	return string(b), nil
}
