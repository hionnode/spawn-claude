package alloy

import (
	"archive/zip"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
)

func releaseURL(version string) string {
	return fmt.Sprintf("https://github.com/grafana/alloy/releases/download/%s/alloy-darwin-arm64.zip", version)
}

// EnsureBinary downloads and installs the pinned Alloy binary at
// /usr/local/bin/alloy if it isn't already present. Extraction happens in a
// user-writable tempdir; the final copy into /usr/local/bin and ownership fix
// require sudo and are funneled through SudoShell.
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

	script := fmt.Sprintf(
		"install -o root -g wheel -m 0755 %q %q && "+
			"xattr -d com.apple.quarantine %q 2>/dev/null || true",
		extracted, paths.BinPath, paths.BinPath,
	)
	return SudoShell(ctx, script)
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
