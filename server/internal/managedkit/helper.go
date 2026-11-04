package managedkit

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/multica-ai/multica/server/internal/tlsx"
)

func Spawn(binary string, args []string, root string) (*exec.Cmd, error) {
	log, err := os.OpenFile(filepath.Join(root, ".managed", "helper.log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		return nil, err
	}
	defer log.Close()
	cmd := exec.Command(binary, args...)
	cmd.Stdout = log
	cmd.Stderr = log
	cmd.Dir = root
	configureProcess(cmd)
	if err = cmd.Start(); err != nil {
		if !retryProcess(cmd, err) {
			return nil, err
		}
		if err = cmd.Start(); err != nil {
			return nil, err
		}
	}
	return cmd, nil
}
func CopyExecutable(from, to string) error {
	in, err := os.Open(from)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(to, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0700)
	if err != nil {
		return err
	}
	_, err = io.Copy(out, in)
	closeErr := out.Close()
	if err != nil {
		return err
	}
	return closeErr
}

type localHealth struct {
	PID     int     `json:"pid"`
	Profile string  `json:"profile"`
	Server  string  `json:"server_url"`
	Version string  `json:"cli_version"`
	Status  string  `json:"status"`
	Managed *Status `json:"managed_client_update"`
}

type acknowledgement struct {
	SHA     string `json:"sha256"`
	Program string `json:"program"`
	PID     int    `json:"pid"`
	Version string `json:"version"`
}

func confirmed(root string, j Journal, h localHealth) bool {
	var ack acknowledgement
	return ReadJSON(filepath.Join(root, ".managed", "ack.json"), &ack) == nil && ack.SHA == j.Offer.SHA256 && ack.Program == j.NewProgram && ack.PID > 0 && ack.PID == h.PID && ack.Version == j.Offer.Version && candidateHealth(j, h)
}

func candidateHealth(j Journal, h localHealth) bool {
	return h.PID > 0 && h.Profile == j.Profile && h.Server == j.ServerURL && h.Version == j.Offer.Version && h.Managed != nil && h.Managed.InstallationID == j.InstallationID
}

func probeHealth(port int) localHealth {
	c := http.Client{Timeout: 2 * time.Second, Transport: &http.Transport{Proxy: nil}}
	resp, err := c.Get(fmt.Sprintf("http://127.0.0.1:%d/health", port))
	if err != nil {
		return localHealth{}
	}
	defer resp.Body.Close()
	var h localHealth
	_ = json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&h)
	return h
}

// RunHelper owns switching independently of both daemon executables. A local
// health response alone is never accepted as a server reconnection.
func RunHelper(ctx context.Context, root string) error {
	return runHelper(ctx, root, 120*time.Second)
}

func runHelper(ctx context.Context, root string, confirmationTimeout time.Duration) error {
	unlock, err := AcquireUpdateLock(root)
	if err != nil {
		return err
	}
	defer unlock()
	j := LoadJournal(root)
	if !RecoveryPending(j) {
		return nil
	}
	if j.Phase == "interrupted" {
		_ = os.Remove(filepath.Join(root, ".managed", "stop.json"))
	}
	i, err := LoadInstallation(root, j.Profile, j.ServerURL)
	if err != nil || i.ID != j.InstallationID || !j.Offer.Valid() {
		return errors.New("invalid recovery identity")
	}
	if j.RootCAFile != "" {
		_ = os.Setenv(tlsx.RootCAEnv, j.RootCAFile)
	}
	if err = tlsx.InstallFromEnv(); err != nil {
		return err
	}
	j.HelperPID = os.Getpid()
	if err = SaveJournal(root, j); err != nil {
		return err
	}
	oldDir, err := ProgramPath(root, j.OldProgram)
	if err != nil {
		return err
	}
	newDir, err := ProgramPath(root, j.NewProgram)
	if err != nil {
		return err
	}
	stopRequested := func() bool {
		var stop struct {
			SHA string `json:"sha256"`
		}
		return ReadJSON(filepath.Join(root, ".managed", "stop.json"), &stop) == nil && stop.SHA == j.Offer.SHA256
	}
	markStopped := func() error {
		j.Phase = "interrupted"
		j.Reason = "用户已停止，重连时继续恢复"
		return SaveJournal(root, j)
	}
	deadline := time.Now().Add(90 * time.Second)
	for probeHealth(j.HealthPort).PID == j.OldPID {
		if time.Now().After(deadline) {
			return errors.New("previous daemon did not stop")
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(200 * time.Millisecond):
		}
	}
	h := probeHealth(j.HealthPort)
	if stopRequested() {
		return markStopped()
	}
	if h.PID != 0 && !candidateHealth(j, h) {
		return errors.New("health port belongs to another daemon")
	}
	j.Phase = "switching"
	_ = SaveJournal(root, j)
	if err = SetProgram(root, j.NewProgram); err != nil {
		return err
	}
	var child *exec.Cmd
	waitingNetwork := h.PID == 0 && !serverReady(j.ServerURL)
	if h.PID == 0 && !waitingNetwork {
		child, err = Spawn(filepath.Join(newDir, "u-workers.exe"), j.Args, root)
	}
	if err != nil {
		return restorePrevious(root, j, oldDir, "新版进程无法启动，已恢复旧版")
	}
	exited := make(chan error, 1)
	if child != nil {
		go func() { exited <- child.Wait() }()
	}
	j.Phase = "verifying"
	_ = SaveJournal(root, j)
	deadline = time.Now().Add(confirmationTimeout)
	for {
		if stopRequested() {
			return markStopped()
		}
		if waitingNetwork {
			j.Reason = "服务器暂不可达，等待恢复后启动并确认新版"
			_ = SaveJournal(root, j)
			if !serverReady(j.ServerURL) {
				select {
				case <-ctx.Done():
					return ctx.Err()
				case <-time.After(3 * time.Second):
					continue
				}
			}
			child, err = Spawn(filepath.Join(newDir, "u-workers.exe"), j.Args, root)
			if err != nil {
				return restorePrevious(root, j, oldDir, "新版进程无法启动，已恢复旧版")
			}
			exited = make(chan error, 1)
			watch := exited
			candidate := child
			go func() { watch <- candidate.Wait() }()
			waitingNetwork = false
			j.Reason = "等待新版鉴权心跳"
			_ = SaveJournal(root, j)
			deadline = time.Now().Add(confirmationTimeout)
		}
		h = probeHealth(j.HealthPort)
		if child != nil && h.PID == child.Process.Pid && (h.Profile != j.Profile || h.Server != j.ServerURL || h.Version != j.Offer.Version) {
			_ = child.Process.Kill()
			<-exited
			return restorePrevious(root, j, oldDir, "新版身份或版本不符，已恢复旧版")
		}
		if confirmed(root, j, h) {
			j.Phase = "completed"
			j.Reason = "新版已重连原服务器并确认版本"
			return SaveJournal(root, j)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-exited:
			if stopRequested() {
				return markStopped()
			}
			if !serverReady(j.ServerURL) {
				child = nil
				waitingNetwork = true
				continue
			}
			return restorePrevious(root, j, oldDir, "新版启动后退出，已恢复旧版")
		case <-time.After(time.Second):
		}
		// An authenticated heartbeat can confirm the pinned program even after
		// a cache withdrawal or server target change. A live but unconfirmed
		// process must also have a bounded recovery path when the server is up.
		if time.Now().After(deadline) {
			if !serverReady(j.ServerURL) {
				continue
			}
			if confirmed(root, j, probeHealth(j.HealthPort)) {
				j.Phase = "completed"
				j.Reason = "新版已重连原服务器并确认版本"
				return SaveJournal(root, j)
			}
			if h.PID != 0 {
				if !candidateHealth(j, h) || !processAt(h.PID, filepath.Join(newDir, "u-workers.exe")) {
					return errors.New("cannot stop an unowned health process")
				}
				p, e := os.FindProcess(h.PID)
				if e != nil {
					return e
				}
				e = p.Kill()
				_ = p.Release()
				if e != nil {
					return e
				}
			} else if child != nil {
				_ = child.Process.Kill()
			}
			if child != nil {
				<-exited
			}
			stopDeadline := time.Now().Add(10 * time.Second)
			for probeHealth(j.HealthPort).PID != 0 {
				if time.Now().After(stopDeadline) {
					return errors.New("successor health port did not close")
				}
				time.Sleep(100 * time.Millisecond)
			}
			return restorePrevious(root, j, oldDir, "新版未完成鉴权确认，已恢复旧版")
		}
	}
}

// This probe only distinguishes an unavailable original server from a local
// executable failure. It never substitutes for the authenticated heartbeat.
func serverReady(origin string) bool {
	client := &http.Client{Timeout: 3 * time.Second, Transport: tlsx.HTTPTransport(), CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := client.Get(strings.TrimRight(origin, "/") + "/readyz")
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}
func restorePrevious(root string, j Journal, oldDir, reason string) error {
	j.Phase = "rolled_back"
	j.Reason = reason
	j.RetryAfter = time.Now().Add(time.Duration(j.Attempts) * 5 * time.Minute)
	if err := SetProgram(root, j.OldProgram); err != nil {
		return err
	}
	if err := SaveJournal(root, j); err != nil {
		return err
	}
	child, err := Spawn(filepath.Join(oldDir, "u-workers.exe"), j.Args, root)
	if err != nil {
		return fmt.Errorf("previous version restart failed: %w", err)
	}
	return child.Process.Release()
}
func OwnExecutable(root string) bool {
	exe, err := os.Executable()
	if err != nil {
		return false
	}
	dir, err := ProgramPath(root, CurrentProgram(root))
	if err != nil {
		return false
	}
	return strings.EqualFold(filepath.Clean(exe), filepath.Join(dir, "u-workers.exe"))
}
