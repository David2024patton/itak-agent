package config

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/David2024patton/iTaKAgent/pkg/debug"
)

// BackupOpts configures what to include in a backup archive.
type BackupOpts struct {
	OnlyConfig  bool   // only include config files, skip memory/session data
	NoWorkspace bool   // exclude workspace files from the archive
	OutputDir   string // directory to write the backup to (default: current dir)
}

// BackupManifest records what was archived and its integrity checksums.
type BackupManifest struct {
	Version   string            `json:"version"`
	CreatedAt string            `json:"created_at"`
	Hostname  string            `json:"hostname"`
	Files     map[string]string `json:"files"` // relative path -> SHA-256
	Opts      BackupOpts        `json:"opts"`
}

// CreateBackup archives config, memory, session data, and optionally
// workspace files into a timestamped .tar.gz.
// Mirrors OpenClaw v2026.3.8: "CLI/backup: add openclaw backup create
// and openclaw backup verify for local state archives."
func CreateBackup(dataDir string, configPath string, opts BackupOpts) (string, error) {
	timestamp := time.Now().Format("2006-01-02-150405")
	hostname, _ := os.Hostname()

	outDir := opts.OutputDir
	if outDir == "" {
		outDir = "."
	}

	archiveName := fmt.Sprintf("itakagent-backup-%s.tar.gz", timestamp)
	archivePath := filepath.Join(outDir, archiveName)

	outFile, err := os.Create(archivePath)
	if err != nil {
		return "", fmt.Errorf("backup: create archive: %w", err)
	}
	defer outFile.Close()

	gzWriter := gzip.NewWriter(outFile)
	defer gzWriter.Close()

	tarWriter := tar.NewWriter(gzWriter)
	defer tarWriter.Close()

	manifest := BackupManifest{
		Version:   "1.0",
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
		Hostname:  hostname,
		Files:     make(map[string]string),
		Opts:      opts,
	}

	// Always include config.
	if configPath != "" {
		if err := addFileToTar(tarWriter, configPath, "config/", &manifest); err != nil {
			debug.Warn("backup", "Skip config %s: %v", configPath, err)
		}
	}

	if !opts.OnlyConfig {
		// Include memory and session data.
		memDirs := []string{"memory", "sessions", "entities", "reflections"}
		for _, dir := range memDirs {
			dirPath := filepath.Join(dataDir, dir)
			if _, err := os.Stat(dirPath); err == nil {
				if err := addDirToTar(tarWriter, dirPath, dir+"/", &manifest); err != nil {
					debug.Warn("backup", "Skip dir %s: %v", dir, err)
				}
			}
		}
	}

	// Write manifest as the last entry.
	manifestJSON, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return "", fmt.Errorf("backup: marshal manifest: %w", err)
	}

	hdr := &tar.Header{
		Name:    "manifest.json",
		Size:    int64(len(manifestJSON)),
		Mode:    0o644,
		ModTime: time.Now(),
	}
	if err := tarWriter.WriteHeader(hdr); err != nil {
		return "", fmt.Errorf("backup: write manifest header: %w", err)
	}
	if _, err := tarWriter.Write(manifestJSON); err != nil {
		return "", fmt.Errorf("backup: write manifest: %w", err)
	}

	debug.Info("backup", "Created backup: %s (%d files)", archivePath, len(manifest.Files))
	return archivePath, nil
}

// VerifyBackup validates a backup archive's manifest and checksums.
func VerifyBackup(archivePath string) error {
	f, err := os.Open(archivePath)
	if err != nil {
		return fmt.Errorf("backup: open archive: %w", err)
	}
	defer f.Close()

	gzReader, err := gzip.NewReader(f)
	if err != nil {
		return fmt.Errorf("backup: invalid gzip: %w", err)
	}
	defer gzReader.Close()

	tarReader := tar.NewReader(gzReader)

	var manifest *BackupManifest
	fileHashes := make(map[string]string)

	for {
		hdr, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("backup: read tar entry: %w", err)
		}

		if hdr.Name == "manifest.json" {
			data, err := io.ReadAll(tarReader)
			if err != nil {
				return fmt.Errorf("backup: read manifest: %w", err)
			}
			manifest = &BackupManifest{}
			if err := json.Unmarshal(data, manifest); err != nil {
				return fmt.Errorf("backup: parse manifest: %w", err)
			}
			continue
		}

		// Hash the file for verification.
		hasher := sha256.New()
		if _, err := io.Copy(hasher, tarReader); err != nil {
			return fmt.Errorf("backup: hash file %s: %w", hdr.Name, err)
		}
		fileHashes[hdr.Name] = hex.EncodeToString(hasher.Sum(nil))
	}

	if manifest == nil {
		return fmt.Errorf("backup: no manifest.json found in archive")
	}

	// Verify checksums.
	mismatches := 0
	for path, expectedHash := range manifest.Files {
		actualHash, found := fileHashes[path]
		if !found {
			debug.Warn("backup", "Missing file in archive: %s", path)
			mismatches++
			continue
		}
		if actualHash != expectedHash {
			debug.Warn("backup", "Checksum mismatch: %s (expected: %s, got: %s)",
				path, expectedHash[:16], actualHash[:16])
			mismatches++
		}
	}

	if mismatches > 0 {
		return fmt.Errorf("backup: %d integrity errors found", mismatches)
	}

	debug.Info("backup", "Verified %s: %d files, all checksums valid", archivePath, len(manifest.Files))
	return nil
}

// addFileToTar adds a single file to the tar archive.
func addFileToTar(tw *tar.Writer, filePath string, prefix string, manifest *BackupManifest) error {
	f, err := os.Open(filePath)
	if err != nil {
		return err
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return err
	}

	hdr, err := tar.FileInfoHeader(info, "")
	if err != nil {
		return err
	}
	hdr.Name = prefix + info.Name()

	if err := tw.WriteHeader(hdr); err != nil {
		return err
	}

	// Hash while copying.
	hasher := sha256.New()
	tee := io.TeeReader(f, hasher)

	if _, err := io.Copy(tw, tee); err != nil {
		return err
	}

	manifest.Files[hdr.Name] = hex.EncodeToString(hasher.Sum(nil))
	return nil
}

// addDirToTar recursively adds all files in a directory.
func addDirToTar(tw *tar.Writer, dirPath string, prefix string, manifest *BackupManifest) error {
	return filepath.Walk(dirPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}

		relPath, err := filepath.Rel(filepath.Dir(dirPath), path)
		if err != nil {
			return err
		}

		hdr, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return err
		}
		hdr.Name = relPath

		if err := tw.WriteHeader(hdr); err != nil {
			return err
		}

		f, err := os.Open(path)
		if err != nil {
			return err
		}
		defer f.Close()

		hasher := sha256.New()
		tee := io.TeeReader(f, hasher)

		if _, err := io.Copy(tw, tee); err != nil {
			return err
		}

		manifest.Files[hdr.Name] = hex.EncodeToString(hasher.Sum(nil))
		return nil
	})
}
