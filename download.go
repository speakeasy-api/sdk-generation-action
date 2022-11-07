package main

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/google/go-github/v48/github"

	"github.com/invopop/yaml"
)

func extract(fileName string, outDir string) error {
	f, err := os.Open(fileName)
	if err != nil {
		return fmt.Errorf("failed to open archive: %w", err)
	}
	defer f.Close()

	uncompressedStream, err := gzip.NewReader(f)
	if err != nil {
		return fmt.Errorf("failed to read archive: %w", err)
	}

	tarReader := tar.NewReader(uncompressedStream)

	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("failed to read tar: %w", err)
		}

		switch header.Typeflag {
		case tar.TypeReg:
			outFile, err := os.Create(path.Join(outDir, header.Name))
			if err != nil {
				return fmt.Errorf("failed to create file from archive: %w", err)
			}
			if _, err := io.Copy(outFile, tarReader); err != nil {
				return fmt.Errorf("failed to copy file from archive: %w", err)
			}

			if err := outFile.Chmod(0o755); err != nil {
				return fmt.Errorf("failed to set file permissions: %w", err)
			}

			outFile.Close()
		default:
			return fmt.Errorf("unsupported type: %v in %s", header.Typeflag, header.Name)
		}
	}

	return nil
}

func downloadFile(url string, file string) (string, error) {
	res, err := http.Get(url)
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

func downloadSpeakeasy(pinnedVersion string) error {
	version := pinnedVersion

	if pinnedVersion == "" || pinnedVersion == "latest" {
		client := github.NewClient(nil)
		tags, _, err := client.Repositories.ListTags(context.Background(), "speakeasy-api", "speakeasy", nil)
		if err != nil {
			return fmt.Errorf("failed to get speakeasy cli tags: %w", err)
		}

		if len(tags) == 0 {
			return fmt.Errorf("no speakeasy cli tags found")
		}

		version = tags[0].GetName()
	} else {
		if !strings.HasPrefix(pinnedVersion, "v") {
			version = "v" + pinnedVersion
		}
	}

	fmt.Println("Downloading speakeasy cli version: ", version)

	speakeasyCLIPath := fmt.Sprintf("https://github.com/speakeasy-api/speakeasy/releases/download/%s/speakeasy_%s_Linux_x86_64.tar.gz", version, strings.TrimPrefix(version, "v"))

	fileName, err := downloadFile(speakeasyCLIPath, "speakeasy*.tar.gz")
	if err != nil {
		return fmt.Errorf("failed to download speakeasy cli: %w", err)
	}

	if err := extract(fileName, filepath.Join(baseDir, "bin")); err != nil {
		return fmt.Errorf("failed to extract speakeasy cli: %w", err)
	}

	os.Remove(fileName)

	return nil
}

func getOpenAPIFileInfo(openAPIPath string) (string, string, string, error) {
	var filePath string

	localPath := filepath.Join(baseDir, "repo", openAPIPath)

	if _, err := os.Stat(localPath); err == nil {
		fmt.Println("Using local OpenAPI file: ", localPath)

		filePath = localPath
	} else {
		u, err := url.Parse(openAPIPath)
		if err != nil {
			return "", "", "", fmt.Errorf("failed to parse openapi url: %w", err)
		}

		fmt.Println("Downloading openapi file from: ", u.String())

		filePath, err = downloadFile(u.String(), "openapi")
		if err != nil {
			return "", "", "", fmt.Errorf("failed to download openapi file: %w", err)
		}
	}

	data, err := os.ReadFile(filePath)
	if err != nil {
		return "", "", "", fmt.Errorf("failed to read openapi file: %w", err)
	}

	var doc struct {
		Info struct {
			Version string
		}
	}

	if err := yaml.Unmarshal(data, &doc); err != nil {
		return "", "", "", fmt.Errorf("failed to parse openapi file: %w", err)
	}

	hash := md5.Sum(data)
	checksum := hex.EncodeToString(hash[:])

	return filePath, checksum, doc.Info.Version, nil
}
