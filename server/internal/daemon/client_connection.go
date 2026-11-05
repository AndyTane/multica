package daemon

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/multica-ai/multica/server/internal/managedkit"
	"github.com/multica-ai/multica/server/pkg/agent"
)

func (d *Daemon) connectionInterval() time.Duration {
	if d.cfg.HeartbeatInterval > 0 {
		return d.cfg.HeartbeatInterval
	}
	return DefaultHeartbeatInterval
}

func (d *Daemon) clientConnectionTick(ctx context.Context, workspaceID string, reconcile bool) (result error) {
	defer func() { d.connectionFailed.Store(result != nil) }()
	var missing []string
	if reconcile {
		providers := append([]string{}, agent.SupportedTypes...)
		for _, r := range agent.BuiltinRuntimes {
			providers = append(providers, r.ID)
		}
		for _, p := range providers {
			_, present := d.agents()[p]
			if !present || (p == "opencode" && d.openCodeDiagnostic(d.agents()[p].Path).State != "configured") || (p == "dsh" && !dshMulticaProfilePresent()) {
				missing = append(missing, p)
			}
		}
	}
	var response struct {
		Status      string            `json:"status"`
		WorkspaceID string            `json:"workspace_id"`
		DaemonID    string            `json:"daemon_id"`
		Offer       *managedkit.Offer `json:"managed_client_update"`
	}
	checkCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	err := d.client.postJSON(checkCtx, "/api/daemon/client-heartbeat", map[string]any{"workspace_id": workspaceID, "daemon_id": d.cfg.DaemonID, "unavailable_builtin_providers": missing}, &response)
	cancel()
	if err != nil {
		var re *requestError
		if errors.As(err, &re) && re.StatusCode == http.StatusNotFound && strings.Contains(re.Body, "404 page not found") {
			// Older servers can verify connection without the new control path.
			// This fallback never acknowledges a managed switch or invents an offer.
			workspaces, e := d.client.ListWorkspaces(ctx)
			if e != nil {
				return e
			}
			for _, ws := range workspaces {
				if ws.ID == workspaceID {
					d.connectionVerified.Store(time.Now().UnixNano())
					return nil
				}
			}
			return fmt.Errorf("workspace access is no longer available")
		}
		return err
	}
	if response.Status != "connected" || response.WorkspaceID != workspaceID || response.DaemonID != d.cfg.DaemonID {
		return fmt.Errorf("invalid client connection receipt")
	}
	d.connectionVerified.Store(time.Now().UnixNano())
	// Reconciliation runs before startup readiness; acknowledging then would
	// let the helper accept a candidate that has not finished startup.
	if d.ready.Load() {
		go d.handleManagedClient(ctx, "", response.Offer)
	}
	return nil
}

func (d *Daemon) clientConnectionLoop(ctx context.Context) {
	tick := func() {
		d.mu.Lock()
		ids := make([]string, 0, len(d.workspaces))
		for id := range d.workspaces {
			ids = append(ids, id)
		}
		d.mu.Unlock()
		if len(ids) == 0 {
			if err := d.syncWorkspacesFromAPI(ctx, false); err == nil {
				d.connectionVerified.Store(time.Now().UnixNano())
				d.connectionFailed.Store(false)
			} else {
				d.connectionFailed.Store(true)
			}
			return
		}
		for _, id := range ids {
			err := d.clientConnectionTick(ctx, id, false)
			if err != nil && ctx.Err() == nil {
				d.logger.Warn("client connection check failed", "workspace_id", id, "error", err)
			}
		}
	}
	tick()
	ticker := time.NewTicker(d.connectionInterval())
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			tick()
		}
	}
}
