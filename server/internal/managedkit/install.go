package managedkit

import (
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type Installation struct {
	ID        string `json:"id"`
	Profile   string `json:"profile"`
	ServerURL string `json:"serverUrl"`
	Paused    bool   `json:"paused"`
}
type Journal struct {
	Offer          Offer     `json:"offer"`
	InstallationID string    `json:"installation_id"`
	Profile        string    `json:"profile"`
	ServerURL      string    `json:"server_url"`
	RuntimeID      string    `json:"runtime_id"`
	Phase          string    `json:"phase"`
	Reason         string    `json:"reason,omitempty"`
	Attempts       int       `json:"attempts"`
	RetryAfter     time.Time `json:"retry_after"`
	OldProgram     string    `json:"old_program"`
	NewProgram     string    `json:"new_program"`
	OldPID         int       `json:"old_pid"`
	HelperPID      int       `json:"helper_pid"`
	Args           []string  `json:"args"`
	HealthPort     int       `json:"health_port"`
	RootCAFile     string    `json:"root_ca_file,omitempty"`
}

func JournalPath(root string) string           { return filepath.Join(root, ".managed", "update.json") }
func LoadJournal(root string) Journal          { var j Journal; _ = ReadJSON(JournalPath(root), &j); return j }
func SaveJournal(root string, j Journal) error { return WriteJSON(JournalPath(root), j) }

type Status struct {
	InstallationID string `json:"installation_id"`
	Phase          string `json:"phase"`
	Version        string `json:"version"`
	Reason         string `json:"reason,omitempty"`
	Attempts       int    `json:"attempts"`
	Paused         bool   `json:"paused"`
}

func Snapshot(root string) Status {
	j := LoadJournal(root)
	var i Installation
	_ = ReadJSON(filepath.Join(root, "kit-install.json"), &i)
	s := Status{InstallationID: i.ID, Phase: j.Phase, Version: j.Offer.Version, Reason: j.Reason, Attempts: j.Attempts, Paused: i.Paused}
	if i.Paused {
		s.Phase = "paused"
	}
	return s
}
func LoadInstallation(root, profile, server string) (*Installation, error) {
	if root == "" {
		return nil, errors.New("not a managed kit")
	}
	root, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	var i Installation
	if err = ReadJSON(filepath.Join(root, "kit-install.json"), &i); err != nil {
		return nil, err
	}
	a, aErr := url.Parse(i.ServerURL)
	b, bErr := url.Parse(server)
	sameServer := aErr == nil && bErr == nil && a.Scheme == "https" && b.Scheme == "https" && a.User == nil && b.User == nil && strings.EqualFold(a.Host, b.Host) && strings.TrimRight(a.Path, "/") == strings.TrimRight(b.Path, "/") && a.RawQuery == "" && b.RawQuery == "" && a.Fragment == "" && b.Fragment == ""
	if len(i.ID) != 36 || i.Profile != profile || !sameServer {
		return nil, errors.New("kit installation identity mismatch")
	}
	return &i, nil
}

// ProgramPath accepts only the original kit or a single immutable version slot.
func ProgramPath(root, relative string) (string, error) {
	if relative == "." {
		return root, nil
	}
	parts := strings.Split(filepath.ToSlash(relative), "/")
	if len(parts) != 2 || parts[0] != "programs" || !regexpSlot(parts[1]) {
		return "", errors.New("invalid program slot")
	}
	for _, path := range []string{root, filepath.Join(root, "programs"), filepath.Join(root, "programs", parts[1])} {
		if info, err := os.Lstat(path); err == nil && info.Mode()&os.ModeSymlink != 0 {
			return "", errors.New("linked program directory")
		}
	}
	return filepath.Join(root, "programs", parts[1]), nil
}
func regexpSlot(s string) bool {
	if len(s) < 1 || len(s) > 160 || s == "." || s == ".." {
		return false
	}
	for _, r := range s {
		if !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '.' || r == '-') {
			return false
		}
	}
	return true
}
func CurrentProgram(root string) string {
	var p struct {
		Program string `json:"program"`
	}
	if ReadJSON(filepath.Join(root, "current.json"), &p) != nil {
		return "."
	}
	if _, err := ProgramPath(root, p.Program); err != nil {
		return "."
	}
	return p.Program
}
func SetProgram(root, relative string) error {
	if _, err := ProgramPath(root, relative); err != nil {
		return err
	}
	return WriteJSON(filepath.Join(root, "current.json"), map[string]string{"program": relative})
}
func Acknowledge(root, profile, server, version string, offer *Offer) bool {
	j := LoadJournal(root)
	if offer == nil || offer.SHA256 != j.Offer.SHA256 {
		return false
	}
	if j.Phase != "switching" && j.Phase != "verifying" {
		return false
	}
	if j.Profile != profile || j.ServerURL != server || j.Offer.Version != version {
		return false
	}
	exe, err := os.Executable()
	if err != nil {
		return false
	}
	expected, err := ProgramPath(root, j.NewProgram)
	if err != nil || !strings.EqualFold(filepath.Clean(exe), filepath.Join(expected, "u-workers.exe")) {
		return false
	}
	return WriteJSON(filepath.Join(root, ".managed", "ack.json"), map[string]any{"sha256": j.Offer.SHA256, "pid": os.Getpid(), "version": version}) == nil
}
func CanAttempt(j Journal, o Offer, now time.Time) bool {
	if j.Offer.SHA256 != o.SHA256 {
		return true
	}
	return j.Phase != "completed" && j.Phase != "switching" && j.Phase != "verifying" && j.Phase != "interrupted" && j.Attempts < 3 && !now.Before(j.RetryAfter)
}
