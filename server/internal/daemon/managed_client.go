package daemon

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/multica-ai/multica/server/internal/cli"
	"github.com/multica-ai/multica/server/internal/managedkit"
	"github.com/multica-ai/multica/server/internal/tlsx"
)

func (d *Daemon) managedClientRoot() string {
	root := os.Getenv("U_WORKERS_KIT_ROOT")
	if runtime.GOOS != "windows" || runtime.GOARCH != "amd64" || d.cfg.LaunchedBy == "desktop" || !cli.IsReleaseVersion(d.cfg.CLIVersion) {
		return ""
	}
	if _, err := managedkit.LoadInstallation(root, d.cfg.Profile, d.cfg.ServerBaseURL); err != nil || !managedkit.OwnExecutable(root) {
		return ""
	}
	return root
}
func (d *Daemon) handleManagedClient(ctx context.Context, runtimeID string, offer *managedkit.Offer) {
	root := d.managedClientRoot()
	if root == "" {
		return
	}
	// Confirmation and the next claim owner must not overlap across heartbeat
	// goroutines, including the interval before the update CAS is acquired.
	if !d.managedClientMu.TryLock() {
		return
	}
	defer d.managedClientMu.Unlock()
	if d.managedVerifying.Load() {
		if managedkit.Acknowledge(root, d.cfg.Profile, d.cfg.ServerBaseURL, d.cfg.CLIVersion) && d.managedVerifying.CompareAndSwap(true, false) {
			d.releaseClaimBarrier()
		} else {
			managedkit.ResumeHelper(root)
			return
		}
	}
	if offer == nil || !offer.Valid() || !cli.IsNewerVersion(offer.Version, d.cfg.CLIVersion) {
		return
	}
	if !d.updating.CompareAndSwap(false, true) {
		return
	}
	handedOff := false
	claimsOwned := false
	defer func() {
		if !handedOff {
			if claimsOwned {
				d.releaseClaimBarrier()
			}
			d.updating.Store(false)
		}
	}()
	unlock, err := managedkit.AcquireUpdateLock(root)
	if err != nil {
		return
	}
	defer func() {
		if unlock != nil {
			unlock()
		}
	}()
	installation, err := managedkit.LoadInstallation(root, d.cfg.Profile, d.cfg.ServerBaseURL)
	if err != nil {
		return
	}
	if installation.Paused {
		return
	}
	previous := managedkit.LoadJournal(root)
	if !managedkit.CanAttempt(previous, *offer, time.Now()) {
		return
	}
	attempts := 1
	if previous.Offer.SHA256 == offer.SHA256 {
		attempts = previous.Attempts + 1
	}
	j := managedkit.Journal{Offer: *offer, InstallationID: installation.ID, Profile: d.cfg.Profile, ServerURL: d.cfg.ServerBaseURL, RuntimeID: runtimeID, Phase: "waiting_idle", Attempts: attempts - 1, OldProgram: managedkit.CurrentProgram(root), OldPID: os.Getpid(), Args: append([]string(nil), os.Args[1:]...), HealthPort: d.cfg.HealthPort}
	j.RootCAFile = tlsx.EnvPath()
	if err = managedkit.SaveJournal(root, j); err != nil {
		return
	}
	// Pause under the same mutex used by every claim, then drain existing work.
	claimsOwned = true
	if !d.drainManagedClaims(ctx, func() bool {
		current, e := managedkit.LoadInstallation(root, d.cfg.Profile, d.cfg.ServerBaseURL)
		return e != nil || current.Paused
	}) {
		j.Phase = "paused"
		_ = managedkit.SaveJournal(root, j)
		return
	}
	j.Phase = "downloading"
	j.Attempts = attempts
	_ = managedkit.SaveJournal(root, j)
	fail := func(reason string) {
		j.Phase = "failed"
		j.Reason = reason
		j.RetryAfter = time.Now().Add(time.Duration(attempts) * 5 * time.Minute)
		_ = managedkit.SaveJournal(root, j)
	}
	work := filepath.Join(root, ".managed", fmt.Sprintf("attempt-%s-%d", offer.SHA256, attempts))
	if err = os.MkdirAll(work, 0700); err != nil {
		fail("无法建立更新目录")
		return
	}
	archive := filepath.Join(work, "client-kit.zip")
	if err = d.downloadManagedClient(ctx, runtimeID, *offer, archive); err != nil {
		fail("下载或校验失败，保留原版；请检查服务器升级缓存与连接")
		return
	}
	j.NewProgram = "programs/" + offer.Version + "-" + offer.SHA256[:12] + fmt.Sprintf("-%d", attempts)
	destination, _ := managedkit.ProgramPath(root, j.NewProgram)
	if err = managedkit.Extract(archive, destination, *offer); err != nil {
		fail("客户端包校验失败，保留原版")
		return
	}
	helper := filepath.Join(work, "helper.exe")
	exe, _ := os.Executable()
	if err = managedkit.CopyExecutable(exe, helper); err != nil {
		fail("恢复助手准备失败，保留原版")
		return
	}
	if latest, e := managedkit.LoadInstallation(root, d.cfg.Profile, d.cfg.ServerBaseURL); e != nil || latest.Paused {
		j.Phase = "paused"
		_ = managedkit.SaveJournal(root, j)
		return
	}
	j.Phase = "switching"
	if err = managedkit.SaveJournal(root, j); err != nil {
		return
	}
	// The persisted phase blocks every new target before releasing the writer
	// lock to the helper. No daemon journal writes are allowed after this point.
	unlock()
	unlock = nil
	cmd, err := managedkit.Spawn(helper, []string{"kit-recover", "--root", root}, root)
	if err != nil {
		// A concurrent explicit recovery may already own the handoff. Only an
		// exclusive writer can abandon it while this old daemon is still alive.
		if release, lockErr := managedkit.AcquireUpdateLock(root); lockErr == nil {
			fail("恢复助手启动失败，保留原版")
			release()
			return
		}
	} else {
		_ = cmd.Process.Release()
	}
	handedOff = true
	// The helper, not the ordinary restart path, owns the successor.
	if d.cancelFunc != nil {
		d.cancelFunc()
	}
}

func (d *Daemon) drainManagedClaims(ctx context.Context, paused func() bool) bool {
	d.claimMu.Lock()
	d.pauseClaims = true
	d.claimMu.Unlock()
	for {
		if paused() {
			return false
		}
		d.claimMu.Lock()
		idle := d.claimsInFlight == 0 && d.activeTasks.Load() == 0
		d.claimMu.Unlock()
		if idle {
			return true
		}
		select {
		case <-ctx.Done():
			return false
		case <-time.After(100 * time.Millisecond):
		}
	}
}
func (d *Daemon) downloadManagedClient(ctx context.Context, runtimeID string, o managedkit.Offer, destination string) error {
	base, err := url.Parse(d.cfg.ServerBaseURL)
	if err != nil || base.Scheme != "https" || base.User != nil {
		return fmt.Errorf("managed updates require HTTPS")
	}
	path := "/api/daemon/client-package?runtime_id=" + url.QueryEscape(runtimeID) + "&sha256=" + o.SHA256
	if runtimeID == "" {
		d.mu.Lock()
		workspaceID := ""
		for id := range d.workspaces {
			workspaceID = id
			break
		}
		d.mu.Unlock()
		if workspaceID == "" {
			return fmt.Errorf("managed updates require an authorized workspace")
		}
		path = "/api/daemon/client-package?workspace_id=" + url.QueryEscape(workspaceID) + "&daemon_id=" + url.QueryEscape(d.cfg.DaemonID) + "&sha256=" + o.SHA256
	}
	downloadCtx, cancel := context.WithTimeout(ctx, 3*time.Minute)
	defer cancel()
	req, err := http.NewRequestWithContext(downloadCtx, http.MethodGet, strings.TrimRight(d.cfg.ServerBaseURL, "/")+path, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+d.client.Token())
	d.client.setIdentityHeaders(req)
	c := *d.client.bundleClient
	c.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	resp, err := c.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK || resp.ContentLength != o.Bytes {
		return fmt.Errorf("package response size or status mismatch")
	}
	f, err := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	_, err = io.Copy(f, io.LimitReader(resp.Body, o.Bytes+1))
	f.Close()
	if err != nil {
		return err
	}
	return managedkit.CheckFile(destination, o.SHA256, o.Bytes)
}
