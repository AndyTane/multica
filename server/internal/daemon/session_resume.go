package daemon

import (
	"os"
	"runtime"

	"github.com/multica-ai/multica/server/pkg/protocol"
)

func (d *Daemon) sessionResumeLauncher() *protocol.SessionResumeLauncher {
	path, err := os.Executable()
	if err != nil {
		return nil
	}
	return &protocol.SessionResumeLauncher{CLIPath: path, Profile: d.cfg.Profile, WorkspacesRoot: d.cfg.WorkspacesRoot, OS: runtime.GOOS}
}
