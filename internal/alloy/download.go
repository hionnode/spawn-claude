package alloy

import (
	"archive/zip"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
)

func releaseURL(version string) string {
	return fmt.Sprintf("https://github.com/grafana/alloy/releases/download/%s/alloy-darwin-arm64.zip", version)
}

// EnsureBinary downloads and installs the pinned Alloy binary if it isn't
// already present at paths.BinPath. Mirrors install.sh L28-L40.
func EnsureBinary(ctx context.Context, paths Paths, version string) error {
	if info, err := os.Stat(paths.BinPath); err == nil && !info.IsDir() && info.Mode()&0o111 != 0 {
		return nil
	}

	tmpDir, err := os.MkdirTemp("", "spawn-claude-alloy-")
	if err != nil {
		return fmt.Errorf("create tempdir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	zipPath := filepath.Join(tmpDir, "alloy.zip")
	if err := downloadFile(ctx, releaseURL(version), zipPath); err != nil {
		return fmt.Errorf("download alloy %s: %w", version, err)
	}

	extracted, err := extractAlloyBinary(zipPath, tmpDir)
	if err != nil {
		return fmt.Errorf("extract alloy zip: %w", err)
	}

	if err := os.Rename(extracted, paths.BinPath); err != nil {
		if err := copyFile(extracted, paths.BinPath); err != nil {
			return fmt.Errorf("install binary to %s: %w", paths.BinPath, err)
		}
	}
	if err := os.Chmod(paths.BinPath, 0o755); err != nil {
		return fmt.Errorf("chmod %s: %w", paths.BinPath, err)
	}

	// Strip quarantine xattr; ignore failure (matches install.sh `|| true`).
	_ = exec.CommandContext(ctx, "xattr", "-d", "com.apple.quarantine", paths.BinPath).Run()
	return nil
}

func downloadFile(ctx context.Context, url, dst string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("GET %s: status %d", url, resp.StatusCode)
	}
	f, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err := io.Copy(f, resp.Body); err != nil {
		return err
	}
	return nil
}

func extractAlloyBinary(zipPath, dstDir string) (string, error) {
	zr, err := zip.OpenReader(zipPath)
	if err != nil {
		return "", err
	}
	defer zr.Close()
	for _, f := range zr.File {
		if f.FileInfo().IsDir() {
			continue
		}
		name := filepath.Base(f.Name)
		if name != "alloy-darwin-arm64" {
			continue
		}
		out := filepath.Join(dstDir, name)
		if err := writeZipFile(f, out); err != nil {
			return "", err
		}
		return out, nil
	}
	return "", fmt.Errorf("alloy-darwin-arm64 not found in zip")
}

func writeZipFile(zf *zip.File, dst string) error {
	rc, err := zf.Open()
	if err != nil {
		return err
	}
	defer rc.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, rc)
	return err
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}
