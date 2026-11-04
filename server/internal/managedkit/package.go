// Package managedkit implements the Windows portable client's release boundary.
package managedkit

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

const MaxPackageBytes int64 = 256 << 20

var releaseVersion = regexp.MustCompile(`^v[0-9]+\.[0-9]+\.[0-9]+$`)
var digest = regexp.MustCompile(`^[a-f0-9]{64}$`)
var sourceCommit = regexp.MustCompile(`^[a-f0-9]{40}$`)

type Offer struct {
	Version  string `json:"version"`
	Commit   string `json:"commit"`
	Platform string `json:"platform"`
	SHA256   string `json:"sha256"`
	Bytes    int64  `json:"bytes"`
}

func (o Offer) Valid() bool {
	return releaseVersion.MatchString(o.Version) && sourceCommit.MatchString(o.Commit) && o.Platform == "windows-amd64" && digest.MatchString(o.SHA256) && o.Bytes > 0 && o.Bytes <= MaxPackageBytes
}

func ReadJSON(path string, out any) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	d := json.NewDecoder(io.LimitReader(f, 1<<20))
	if err := d.Decode(out); err != nil {
		return err
	}
	if err := d.Decode(new(any)); err != io.EOF {
		return errors.New("trailing or oversized JSON")
	}
	return nil
}

func WriteJSON(path string, value any) error {
	b, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	if err = os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".journal-*")
	if err != nil {
		return err
	}
	name := f.Name()
	defer os.Remove(name)
	if _, err = f.Write(b); err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	return os.Rename(name, path)
}

func CheckFile(path, hash string, size int64) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	stat, err := f.Stat()
	if err != nil {
		return err
	}
	if !stat.Mode().IsRegular() || stat.Size() != size || !digest.MatchString(hash) {
		return errors.New("package size mismatch")
	}
	h := sha256.New()
	if _, err = io.Copy(h, f); err != nil {
		return err
	}
	if hex.EncodeToString(h.Sum(nil)) != hash {
		return errors.New("package SHA-256 mismatch")
	}
	return nil
}

var programFiles = map[string]bool{
	"Connect.cmd": true, "Reconnect.cmd": true, "Status.cmd": true, "Stop.cmd": true, "Import.cmd": true,
	"client.ps1": true, "client-settings.ps1": true, "connection.json": true, "README.md": true, "u-workers.exe": true, "multica.exe": true,
}

type File struct {
	Path   string `json:"path"`
	Bytes  int64  `json:"bytes"`
	SHA256 string `json:"sha256"`
}
type Inventory struct {
	SchemaVersion int    `json:"schemaVersion"`
	Version       string `json:"version"`
	Commit        string `json:"commit"`
	Platform      string `json:"platform"`
	Files         []File `json:"files"`
}

// Extract validates the entire archive before exposing any executable. Only the
// fixed distribution allowlist is accepted; user settings are an empty template.
func Extract(archive, destination string, offer Offer) error {
	if !offer.Valid() {
		return errors.New("invalid client release")
	}
	if err := CheckFile(archive, offer.SHA256, offer.Bytes); err != nil {
		return err
	}
	z, err := zip.OpenReader(archive)
	if err != nil {
		return err
	}
	defer z.Close()
	entries := map[string]*zip.File{}
	var expanded uint64
	for _, f := range z.File {
		if f.Name != "CLIENT-FILES.json" && !programFiles[f.Name] {
			return fmt.Errorf("unlisted ZIP path: %q", f.Name)
		}
		if entries[f.Name] != nil || !f.Mode().IsRegular() {
			return errors.New("duplicate or non-regular ZIP entry")
		}
		expanded += f.UncompressedSize64
		if expanded > uint64(MaxPackageBytes) {
			return errors.New("expanded package too large")
		}
		entries[f.Name] = f
	}
	invFile := entries["CLIENT-FILES.json"]
	if invFile == nil {
		return errors.New("missing inventory")
	}
	r, err := invFile.Open()
	if err != nil {
		return err
	}
	var inv Inventory
	err = json.NewDecoder(io.LimitReader(r, 1<<20)).Decode(&inv)
	r.Close()
	if err != nil || inv.SchemaVersion != 2 || inv.Version != offer.Version || inv.Commit != offer.Commit || inv.Platform != offer.Platform || len(inv.Files) != len(programFiles) {
		return errors.New("client inventory identity mismatch")
	}
	if _, err = os.Stat(destination); !os.IsNotExist(err) {
		return errors.New("use a fresh program directory")
	}
	if err = os.MkdirAll(destination, 0700); err != nil {
		return err
	}
	seen := map[string]bool{}
	for _, entry := range inv.Files {
		f := entries[entry.Path]
		if !programFiles[entry.Path] || seen[entry.Path] || f == nil || entry.Bytes < 0 || uint64(entry.Bytes) != f.UncompressedSize64 {
			return errors.New("invalid inventory file")
		}
		seen[entry.Path] = true
		src, err := f.Open()
		if err != nil {
			return err
		}
		out, err := os.OpenFile(filepath.Join(destination, entry.Path), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0700)
		if err != nil {
			src.Close()
			return err
		}
		_, err = io.Copy(out, io.LimitReader(src, MaxPackageBytes+1))
		out.Close()
		src.Close()
		if err != nil {
			return err
		}
		if err = CheckFile(filepath.Join(destination, entry.Path), entry.SHA256, entry.Bytes); err != nil {
			return err
		}
	}
	if len(entries) != len(programFiles)+1 {
		return errors.New("extra archive entries")
	}
	var connection map[string]string
	if err = ReadJSON(filepath.Join(destination, "connection.json"), &connection); err != nil || len(connection) != 3 || connection["profile"] != "" || connection["serverUrl"] != "" || connection["rootCa"] != "" {
		return errors.New("published connection template must be empty")
	}
	for _, key := range []string{"profile", "serverUrl", "rootCa"} {
		if _, ok := connection[key]; !ok {
			return errors.New("invalid connection template")
		}
	}
	var primary File
	for _, f := range inv.Files {
		if f.Path == "u-workers.exe" {
			primary = f
		}
	}
	if err = CheckFile(filepath.Join(destination, "multica.exe"), primary.SHA256, primary.Bytes); err != nil {
		return errors.New("CLI aliases differ")
	}
	return WriteJSON(filepath.Join(destination, "CLIENT-FILES.json"), inv)
}

// ActiveOffer ties readiness to the running API build, so restarting an old
// server after rollback cannot advertise the failed new target.
func ActiveOffer(cache, version, commit string) *Offer {
	if cache == "" || !releaseVersion.MatchString(version) || len(commit) < 9 {
		return nil
	}
	var offer Offer
	directory := filepath.Join(cache, version)
	if ReadJSON(filepath.Join(directory, "ready.json"), &offer) != nil || !offer.Valid() || offer.Version != version || !strings.HasPrefix(offer.Commit, commit) {
		return nil
	}
	info, err := os.Stat(filepath.Join(directory, "client-kit.zip"))
	if err != nil || info.Size() != offer.Bytes {
		return nil
	}
	return &offer
}
