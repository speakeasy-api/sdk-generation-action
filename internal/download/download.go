package download

import (
	"fmt"
	"io"
	"net/http"
	"os"
)

func DownloadFile(url string, file string, header string, token string) (string, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	if header != "" {
		if token == "" {
			return "", fmt.Errorf("token required for header")
		}
		req.Header.Add(header, token)
	}

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to download file: %w", err)
	}
	defer res.Body.Close()

	out, err := os.CreateTemp(os.TempDir(), file)
	if err != nil {
		return "", fmt.Errorf("failed to create temp file for download: %w", err)
	}
	defer out.Close()

	fileName := out.Name()

	if _, err := io.Copy(out, res.Body); err != nil {
		return "", fmt.Errorf("failed to copy file to temp location: %w", err)
	}

	return fileName, nil
}
