package cli

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"

	"github.com/speakeasy-api/sdk-generation-action/internal/download"
	"github.com/speakeasy-api/sdk-generation-action/internal/environment"
)

type Git interface {
	GetLatestTag() (string, error)
	GetDownloadLink(version string) (string, string, error)
}

func Download(pinnedVersion string, g Git) error {
	if pinnedVersion == "" {
		pinnedVersion = "latest"
	}

	version := pinnedVersion

	if pinnedVersion != "latest" {
		if !strings.HasPrefix(pinnedVersion, "v") {
			version = "v" + pinnedVersion
		}
	}

	link, version, err := g.GetDownloadLink(version)
	if err != nil {
		return err
	}

	fmt.Println("Downloading speakeasy cli version: ", version)

	downloadPath := filepath.Join(os.TempDir(), "speakeasy*"+path.Ext(link))
	if err := download.DownloadFile(link, downloadPath, "", ""); err != nil {
		return fmt.Errorf("failed to download speakeasy cli: %w", err)
	}
	defer os.Remove(downloadPath)

	baseDir := environment.GetBaseDir()

	if err := extract(downloadPath, filepath.Join(baseDir, "bin")); err != nil {
		return fmt.Errorf("failed to extract speakeasy cli: %w", err)
	}

	if err := os.Chmod(filepath.Join(baseDir, "bin", "speakeasy"), 0o755); err != nil {
		return fmt.Errorf("failed to set permissions on speakeasy cli: %w", err)
	}

	return nil
}

func runSpeakeasyCommand(args ...string) (string, error) {
	baseDir := environment.GetBaseDir()

	cmdPath := path.Join(baseDir, "bin", "speakeasy")

	output, err := exec.Command(cmdPath, args...).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("error running speakeasy command: speakeasy %s - %w\n %s", strings.Join(args, " "), err, string(output))
	}

	return string(output), nil
}

func extract(archive, dest string) error {
	if err := os.MkdirAll(dest, os.ModePerm); err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}

	switch filepath.Ext(archive) {
	case ".zip":
		return extractZip(archive, dest)
	case ".gz":
		return extractTarGZ(archive, dest)
	default:
		return fmt.Errorf("unsupported archive type: %s", filepath.Ext(archive))
	}
}

func extractZip(archive, dest string) error {
	z, err := zip.OpenReader(archive)
	if err != nil {
		return fmt.Errorf("failed to open zip archive: %w", err)
	}
	defer z.Close()

	for _, file := range z.File {
		filePath := path.Join(dest, file.Name)

		if file.FileInfo().IsDir() {
			if err := os.MkdirAll(filePath, os.ModePerm); err != nil {
				return fmt.Errorf("failed to create extracted directory: %s - %w", filePath, err)
			}
			continue
		}

		if err := os.MkdirAll(path.Dir(filePath), os.ModePerm); err != nil {
			return fmt.Errorf("failed to create extracted directory: %s - %w", path.Dir(filePath), err)
		}

		outFile, err := os.Create(filePath)
		if err != nil {
			return fmt.Errorf("failed to create extracted file: %s - %w", filePath, err)
		}

		f, err := file.Open()
		if err != nil {
			return fmt.Errorf("failed to open file in archive: %w", err)
		}

		_, err = io.Copy(outFile, f)
		f.Close()
		outFile.Close()
		if err != nil {
			return fmt.Errorf("failed to copy file from archive: %w", err)
		}
	}

	return nil
}

func extractTarGZ(archive, dest string) error {
	file, err := os.OpenFile(archive, os.O_RDONLY, 0)
	if err != nil {
		return fmt.Errorf("failed to open archive: %w", err)
	}
	defer file.Close()

	gz, err := gzip.NewReader(file)
	if err != nil {
		return fmt.Errorf("failed to create gzip reader: %w", err)
	}

	t := tar.NewReader(gz)

	for {
		header, err := t.Next()

		if err == io.EOF {
			break
		}

		if err != nil {
			return fmt.Errorf("failed to read archive: %w", err)
		}

		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(path.Join(dest, header.Name), os.ModePerm); err != nil {
				return fmt.Errorf("failed to create extracted directory: %s - %w", path.Join(dest, header.Name), err)
			}
		case tar.TypeReg:
			outFile, err := os.Create(path.Join(dest, header.Name))
			if err != nil {
				return fmt.Errorf("failed to create extracted file: %s - %w", path.Join(dest, header.Name), err)
			}
			_, err = io.Copy(outFile, t)
			outFile.Close()
			if err != nil {
				return fmt.Errorf("failed to copy file from archive: %w", err)
			}
		default:
			return fmt.Errorf("unknown type: %b in %s", header.Typeflag, header.Name)
		}
	}

	return nil
}
