package cmd

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/samber/oops"
	"github.com/spf13/cobra"
)

// repoSlug is the GitHub repository used to resolve releases for self-update.
const repoSlug = "SalvucciFacundo/go-arch"

// httpClient is the shared client for self-update network calls.
var httpClient = &http.Client{Timeout: 60 * time.Second}

var updateCmd = &cobra.Command{
	Use:   "update",
	Short: "Update go-arch to the latest release",
	Long: `Downloads the latest go-arch release, verifies its SHA-256 checksum,
and replaces the currently running binary in place. If the running binary is
already the latest version, it reports that and exits.

The binary is replaced atomically (temp file + rename). The original binary
path is resolved from /proc/self/exe (Linux/macOS) or the running executable
(Windows). If the current binary cannot be written (e.g. /usr/local/bin
without write permission), the command prints the exact sudo command to run.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		exe, err := currentExecutable()
		if err != nil {
			return oops.Code("update_exe_resolve").Wrap(err)
		}

		uiInfo := func(format string, a ...interface{}) {
			fmt.Fprintf(cmd.OutOrStdout(), format+"\n", a...)
		}

		// 1. Resolve the latest release tag from the GitHub API.
		uiInfo("Checking for updates...")
		tag, err := fetchLatestTag()
		if err != nil {
			return oops.Code("update_resolve").Wrap(err)
		}
		version := strings.TrimPrefix(tag, "v")

		// 2. Compare against the running version.
		if Version != "dev" && strings.TrimPrefix(Version, "v") == version {
			uiInfo("go-arch is already up to date (v%s).", version)
			return nil
		}

		// 3. Detect platform and build asset names.
		osName, archName, err := detectPlatform()
		if err != nil {
			return err
		}
		base := fmt.Sprintf("go-arch_%s_%s_%s", version, osName, archName)
		var assetName, checksumName, downloadURL, checksumURL string
		switch osName {
		case "windows":
			assetName = base + ".zip"
		default:
			assetName = base + ".tar.gz"
		}
		checksumName = fmt.Sprintf("go-arch_%s_checksums.txt", version)
		releaseURL := fmt.Sprintf("https://github.com/%s/releases/download/%s", repoSlug, tag)
		downloadURL = releaseURL + "/" + assetName
		checksumURL = releaseURL + "/" + checksumName

		// 4. Download the tarball/zip and the checksums file.
		uiInfo("Downloading %s...", assetName)
		assetData, err := downloadFile(downloadURL)
		if err != nil {
			return oops.Code("update_download").Wrapf(err, "failed to download %s", assetName)
		}
		checksumsData, err := downloadFile(checksumURL)
		if err != nil {
			return oops.Code("update_download").Wrapf(err, "failed to download %s", checksumName)
		}

		// 5. Verify the SHA-256 checksum.
		expected, err := findChecksum(checksumsData, assetName)
		if err != nil {
			return oops.Code("update_checksum").Wrap(err)
		}
		actual := sha256.Sum256(assetData)
		actualHex := hex.EncodeToString(actual[:])
		if !strings.EqualFold(expected, actualHex) {
			return oops.
				Code("update_checksum").
				Hint("Refusing to replace the binary with a corrupted download").
				Errorf("checksum mismatch: expected %s, got %s", expected, actualHex)
		}
		uiInfo("Checksum verified.")

		// 6. Extract the new binary into a temp file next to the current one.
		newBin, err := extractBinary(assetData, assetName)
		if err != nil {
			return oops.Code("update_extract").Wrap(err)
		}
		defer os.Remove(newBin)

		// 7. Atomically replace the running binary.
		if err := replaceBinary(newBin, exe); err != nil {
			return err
		}

		uiInfo("✅ go-arch updated to v%s.", version)
		return nil
	},
}

func init() {
	RootCmd.AddCommand(updateCmd)
}

// currentExecutable returns the absolute path of the running binary.
func currentExecutable() (string, error) {
	// On Linux, /proc/self/exe resolves the real path even when the binary
	// was replaced (important for repeated updates).
	if runtime.GOOS == "linux" {
		if p, err := os.Readlink("/proc/self/exe"); err == nil {
			return p, nil
		}
	}
	return os.Executable()
}

// fetchLatestTag resolves the latest release tag from the GitHub API.
func fetchLatestTag() (string, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/releases/latest", repoSlug)
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "go-arch-update")
	resp, err := httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("GitHub API returned %s", resp.Status)
	}

	var release struct {
		TagName string `json:"tag_name"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return "", err
	}
	if release.TagName == "" {
		return "", fmt.Errorf("release response has no tag_name")
	}
	return release.TagName, nil
}

// detectPlatform maps runtime.GOOS/GOARCH to the goreleaser asset names.
func detectPlatform() (osName, archName string, err error) {
	switch runtime.GOOS {
	case "linux":
		osName = "linux"
	case "darwin":
		osName = "darwin"
	case "windows":
		osName = "windows"
	default:
		return "", "", oops.
			Code("update_unsupported_platform").
			Errorf("self-update is not supported on %s", runtime.GOOS)
	}
	switch runtime.GOARCH {
	case "amd64":
		archName = "amd64"
	case "arm64":
		archName = "arm64"
	default:
		return "", "", oops.
			Code("update_unsupported_platform").
			Errorf("self-update is not supported on %s/%s", runtime.GOOS, runtime.GOARCH)
	}
	return osName, archName, nil
}

// downloadFile fetches a URL into memory.
func downloadFile(url string) ([]byte, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "go-arch-update")
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %s", resp.Status)
	}
	return io.ReadAll(resp.Body)
}

// findChecksum extracts the expected SHA-256 for assetName from a checksums file.
func findChecksum(data []byte, assetName string) (string, error) {
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 2 && fields[1] == assetName {
			return fields[0], nil
		}
	}
	return "", fmt.Errorf("checksum for %s not found in checksums file", assetName)
}

// extractBinary extracts the go-arch binary from a tarball (unix) or zip
// (windows) into a temp file and returns its path.
func extractBinary(data []byte, assetName string) (string, error) {
	tmp, err := os.CreateTemp("", "go-arch-update-*")
	if err != nil {
		return "", err
	}
	tmpPath := tmp.Name()
	defer tmp.Close()

	if strings.HasSuffix(assetName, ".zip") {
		return extractZipBinary(data, tmpPath)
	}
	return extractTarGzBinary(data, tmpPath)
}

func extractTarGzBinary(data []byte, tmpPath string) (string, error) {
	gz, err := gzip.NewReader(strings.NewReader(string(data)))
	if err != nil {
		return "", err
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", err
		}
		if hdr.Typeflag == tar.TypeReg && filepath.Base(hdr.Name) == "go-arch" {
			if err := writeTempExecutable(tmpPath, tr); err != nil {
				return "", err
			}
			return tmpPath, nil
		}
	}
	return "", fmt.Errorf("go-arch binary not found in tarball")
}

func extractZipBinary(data []byte, tmpPath string) (string, error) {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return "", err
	}
	for _, f := range zr.File {
		if filepath.Base(f.Name) == "go-arch.exe" {
			rc, err := f.Open()
			if err != nil {
				return "", err
			}
			defer rc.Close()
			if err := writeTempExecutable(tmpPath, rc); err != nil {
				return "", err
			}
			return tmpPath, nil
		}
	}
	return "", fmt.Errorf("go-arch.exe not found in zip")
}

// writeTempExecutable writes a reader into an executable temp file.
func writeTempExecutable(tmpPath string, r io.Reader) error {
	out, err := os.OpenFile(tmpPath, os.O_WRONLY|os.O_TRUNC, 0o755)
	if err != nil {
		return err
	}
	defer out.Close()
	if _, err := io.Copy(out, r); err != nil {
		return err
	}
	return out.Sync()
}

// replaceBinary atomically replaces exe with newBin, preserving permissions.
func replaceBinary(newBin, exe string) error {
	// Preserve the original mode (e.g. 0755 in /usr/local/bin).
	fi, err := os.Stat(exe)
	if err != nil {
		return oops.Code("update_replace").Wrap(err)
	}
	if err := os.Chmod(newBin, fi.Mode().Perm()); err != nil {
		return oops.Code("update_replace").Wrap(err)
	}

	if err := os.Rename(newBin, exe); err != nil {
		// Rename can fail when the target dir is not writable (e.g.
		// /usr/local/bin without sudo). Give the user the exact command.
		return oops.
			Code("update_permission").
			Hint(fmt.Sprintf("Run: sudo %s update", exe)).
			Wrapf(err, "cannot replace %s", exe)
	}
	return nil
}
