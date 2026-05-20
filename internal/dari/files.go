package dari

import (
	"bytes"
	"context"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
)

type UploadedFile struct {
	ID        string `json:"id"`
	Filename  string `json:"filename"`
	SizeBytes int64  `json:"size_bytes"`
}

func (c *Client) UploadFile(ctx context.Context, path string) (UploadedFile, error) {
	f, err := os.Open(path)
	if err != nil {
		return UploadedFile{}, err
	}
	defer f.Close()
	return c.UploadReader(ctx, filepath.Base(path), f)
}

func (c *Client) UploadReader(ctx context.Context, filename string, r io.Reader) (UploadedFile, error) {
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	part, err := mw.CreateFormFile("file", filepath.Base(filename))
	if err != nil {
		return UploadedFile{}, err
	}
	if _, err := io.Copy(part, r); err != nil {
		return UploadedFile{}, err
	}
	if err := mw.Close(); err != nil {
		return UploadedFile{}, err
	}
	var out UploadedFile
	err = c.doJSON(ctx, http.MethodPost, "/v1/files", mw.FormDataContentType(), &body, &out)
	return out, err
}
