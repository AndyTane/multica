package protocol

// SessionResumeLauncher contains no credentials and is used only for explicit clipboard actions.
type SessionResumeLauncher struct {
	CLIPath        string `json:"cli_path"`
	Profile        string `json:"profile"`
	WorkspacesRoot string `json:"workspaces_root"`
	OS             string `json:"os"`
}
