package cli

import (
	"archive/tar"
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
	GetLatestDownloadLink() (string, string, error)
}

func Download(pinnedVersion string, g Git) error {
	version := pinnedVersion

	var link string

	if pinnedVersion == "" || pinnedVersion == "latest" {
		var err error
		link, version, err = g.GetLatestDownloadLink()
		if err != nil {
			return err
		}
	} else {
		if !strings.HasPrefix(pinnedVersion, "v") {
			version = "v" + pinnedVersion
		}

		link = fmt.Sprintf("https://github.com/speakeasy-api/speakeasy/releases/download/%s/speakeasy_%s_Linux_x86_64.tar.gz", version, strings.TrimPrefix(version, "v"))
	}

	fmt.Println("Downloading speakeasy cli version: ", version)

	tarPath := filepath.Join(os.TempDir(), "speakeasy*.tar.gz")
	if err := download.DownloadFile(link, tarPath, "", ""); err != nil {
		return fmt.Errorf("failed to download speakeasy cli: %w", err)
	}

	baseDir := environment.GetBaseDir()

	if err := extract(tarPath, filepath.Join(baseDir, "bin")); err != nil {
		return fmt.Errorf("failed to extract speakeasy cli: %w", err)
	}

	os.Remove(tarPath)

	return nil
}

func runSpeakeasyCommand(args ...string) (string, error) {
	baseDir := environment.GetBaseDir()

	cmdPath := strings.Join([]string{baseDir, "bin", "speakeasy"}, string(os.PathSeparator))

	output, err := exec.Command(cmdPath, args...).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("error running speakeasy command: speakeasy %s - %w\n %s", strings.Join(args, " "), err, string(output))
	}

	return string(output), nil
}

func extract(fileName string, outDir string) error {
	if err := os.MkdirAll(outDir, os.ModePerm); err != nil {
		return fmt.Errorf("failed to create output directory: %s - %w", outDir, err)
	}

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
