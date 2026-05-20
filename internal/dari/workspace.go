package dari

import (
	"archive/zip"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

const DefaultWorkspaceZipMaxUncompressedBytes int64 = 100 * 1024 * 1024

func (c *Client) DownloadWorkspaceZip(ctx context.Context, sessionID string, paths []string, outPath string) error {
	return c.DownloadWorkspaceZipWithLimit(ctx, sessionID, paths, outPath, 0)
}

func (c *Client) DownloadWorkspaceZipWithLimit(
	ctx context.Context,
	sessionID string,
	paths []string,
	outPath string,
	maxBytes int64,
) error {
	req, err := c.newWorkspaceZipRequest(ctx, sessionID, paths)
	if err != nil {
		return err
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if err := checkWorkspaceZipResponse(resp); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
		return err
	}
	f, err := os.Create(outPath)
	if err != nil {
		return err
	}
	defer f.Close()
	reader := limitReader(resp.Body, maxBytes)
	n, err := io.Copy(f, reader)
	if err == nil && maxBytes > 0 && n > maxBytes {
		_ = f.Close()
		_ = os.Remove(outPath)
		return fmt.Errorf("download workspace exceeds size limit of %d bytes", maxBytes)
	}
	return err
}

func (c *Client) WriteWorkspaceZipWithLimit(ctx context.Context, sessionID string, paths []string, w io.Writer, maxBytes int64) error {
	req, err := c.newWorkspaceZipRequest(ctx, sessionID, paths)
	if err != nil {
		return err
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if err := checkWorkspaceZipResponse(resp); err != nil {
		return err
	}
	reader := limitReader(resp.Body, maxBytes)
	n, err := io.Copy(w, reader)
	if err == nil && maxBytes > 0 && n > maxBytes {
		return fmt.Errorf("download workspace exceeds size limit of %d bytes", maxBytes)
	}
	return err
}

func (c *Client) newWorkspaceZipRequest(ctx context.Context, sessionID string, paths []string) (*http.Request, error) {
	u := "/v1/sessions/" + url.PathEscape(sessionID) + "/workspace.zip"
	if len(paths) > 0 {
		q := url.Values{}
		for _, p := range paths {
			q.Add("path", p)
		}
		u += "?" + q.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.APIKey)
	return req, nil
}

func checkWorkspaceZipResponse(resp *http.Response) error {
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	return fmt.Errorf("download workspace: http %d: %s", resp.StatusCode, strings.TrimSpace(string(b)))
}

func limitReader(r io.Reader, maxBytes int64) io.Reader {
	if maxBytes <= 0 {
		return r
	}
	return io.LimitReader(r, maxBytes+1)
}

func ExtractZip(zipPath, dest string) error {
	return ExtractZipWithLimit(zipPath, dest, DefaultWorkspaceZipMaxUncompressedBytes)
}

func ExtractZipWithLimit(zipPath, dest string, maxUncompressedBytes int64) error {
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return err
	}
	defer r.Close()
	absDest, err := filepath.Abs(dest)
	if err != nil {
		return err
	}
	var total int64
	for _, f := range r.File {
		clean := filepath.Clean(f.Name)
		if clean == "." || strings.HasPrefix(clean, "../") || filepath.IsAbs(clean) {
			return fmt.Errorf("unsafe zip path %q", f.Name)
		}
		outPath := filepath.Join(absDest, clean)
		if !strings.HasPrefix(outPath, absDest+string(os.PathSeparator)) && outPath != absDest {
			return fmt.Errorf("zip path escapes dest: %q", f.Name)
		}
		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(outPath, 0o755); err != nil {
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
			return err
		}
		rc, err := f.Open()
		if err != nil {
			return err
		}
		w, err := os.OpenFile(outPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, f.FileInfo().Mode())
		if err != nil {
			rc.Close()
			return err
		}
		reader := io.Reader(rc)
		if maxUncompressedBytes > 0 {
			remaining := maxUncompressedBytes - total
			if remaining < 0 {
				remaining = 0
			}
			reader = io.LimitReader(rc, remaining+1)
		}
		n, copyErr := io.Copy(w, reader)
		total += n
		closeErr := w.Close()
		rc.Close()
		if copyErr != nil {
			return copyErr
		}
		if maxUncompressedBytes > 0 && total > maxUncompressedBytes {
			_ = os.Remove(outPath)
			return fmt.Errorf("zip exceeds uncompressed size limit of %d bytes", maxUncompressedBytes)
		}
		if closeErr != nil {
			return closeErr
		}
	}
	return nil
}
