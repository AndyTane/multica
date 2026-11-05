package main

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/spf13/cobra"

	"github.com/multica-ai/multica/server/internal/brand"
	"github.com/multica-ai/multica/server/internal/cli"
	"github.com/multica-ai/multica/server/internal/daemon/execenv"
	"github.com/multica-ai/multica/server/internal/envcompat"
	"github.com/multica-ai/multica/server/internal/tlsx"
)

var (
	version = "dev"
	commit  = "unknown"
	date    = "unknown"
)

// debugFlag is bound to the persistent --debug flag and, when set, makes
// FormatError emit the full original error chain instead of just the
// user-facing message.
var debugFlag bool

// cliName is the command name this process presents to users. Current releases
// install both u-workers and the legacy multica executable from the same
// binary; help, version output, and suggestions follow the name it was invoked
// as, so neither installation looks like a renamed leftover.
var cliName = brand.CLICommandFor(filepath.Base(os.Args[0]))

var rootCmd = &cobra.Command{
	Use:           cliName,
	Short:         brand.DisplayName + " CLI — local agent runtime and management tool",
	Long:          "Work seamlessly with " + brand.DisplayName + " from the command line.",
	SilenceUsage:  true,
	SilenceErrors: true,
}

func init() {
	rootCmd.Version = fmt.Sprintf("%s (commit: %s, built: %s)\ngo: %s, os/arch: %s/%s", version, commit, date, runtime.Version(), runtime.GOOS, runtime.GOARCH)
	rootCmd.SetVersionTemplate(cliName + " {{.Version}}\n")

	// Tag every CLI HTTP request with this binary's build version so the
	// server can split logs/metrics by client version.
	cli.ClientVersion = version

	rootCmd.PersistentFlags().String("server-url", "", "U Workers server URL (env: U_WORKERS_SERVER_URL, legacy MULTICA_SERVER_URL)")
	rootCmd.PersistentFlags().String("workspace-id", "", "Workspace ID (env: U_WORKERS_WORKSPACE_ID, legacy MULTICA_WORKSPACE_ID)")
	rootCmd.PersistentFlags().String("profile", "", "Configuration profile name (e.g. dev) — isolates config, daemon state, and workspaces")
	rootCmd.PersistentFlags().BoolVar(&debugFlag, "debug", false, "Print full error details on failure (env: U_WORKERS_DEBUG, legacy MULTICA_DEBUG)")

	// Core commands
	issueCmd.GroupID = groupCore
	projectCmd.GroupID = groupCore
	labelCmd.GroupID = groupCore
	propertyCmd.GroupID = groupCore
	agentCmd.GroupID = groupCore
	autopilotCmd.GroupID = groupCore
	workspaceCmd.GroupID = groupCore
	repoCmd.GroupID = groupCore
	skillCmd.GroupID = groupCore
	squadCmd.GroupID = groupCore
	chatCmd.GroupID = groupCore

	// Runtime commands
	daemonCmd.GroupID = groupRuntime
	runtimeCmd.GroupID = groupRuntime

	// Additional commands
	authCmd.GroupID = groupAdditional
	userCmd.GroupID = groupAdditional
	loginCmd.GroupID = groupAdditional
	setupCmd.GroupID = groupAdditional
	connectCmd.GroupID = groupAdditional
	attachmentCmd.GroupID = groupAdditional
	configCmd.GroupID = groupAdditional
	updateCmd.GroupID = groupAdditional
	versionCmd.GroupID = groupAdditional

	rootCmd.AddCommand(issueCmd)
	rootCmd.AddCommand(projectCmd)
	rootCmd.AddCommand(labelCmd)
	rootCmd.AddCommand(propertyCmd)
	rootCmd.AddCommand(agentCmd)
	rootCmd.AddCommand(autopilotCmd)
	rootCmd.AddCommand(workspaceCmd)
	rootCmd.AddCommand(repoCmd)
	rootCmd.AddCommand(skillCmd)
	rootCmd.AddCommand(squadCmd)
	rootCmd.AddCommand(chatCmd)
	rootCmd.AddCommand(daemonCmd)
	rootCmd.AddCommand(runtimeCmd)
	rootCmd.AddCommand(authCmd)
	rootCmd.AddCommand(userCmd)
	rootCmd.AddCommand(loginCmd)
	rootCmd.AddCommand(setupCmd)
	rootCmd.AddCommand(connectCmd)
	rootCmd.AddCommand(attachmentCmd)
	rootCmd.AddCommand(configCmd)
	rootCmd.AddCommand(updateCmd)
	rootCmd.AddCommand(versionCmd)

	initHelp(rootCmd)

	// Every command that talks to a self-hosted server needs the private root CA
	// persisted by `connect --root-ca`; load it once here instead of in each
	// HTTP client constructor.
	rootCmd.PersistentPreRunE = func(cmd *cobra.Command, _ []string) error {
		bootstrapClientTrust(cmd)
		return nil
	}
}

// bootstrapClientTrust makes a self-hosted server's private root CA available to
// every client this process creates. An explicitly exported U_WORKERS_ROOT_CA_FILE
// / MULTICA_ROOT_CA_FILE wins over the value persisted in the profile config.
//
// A broken CA file is reported but never fatal here: the commands that write the
// config (including `config set root_ca_file ""`, the documented way out) must
// still run. Commands that depend on the trust anchor fail loudly on their own
// when the TLS handshake cannot be verified.
func bootstrapClientTrust(cmd *cobra.Command) {
	// A surviving task marker without its private root must never make trust
	// bootstrap discover the owner's profile before the API identity guard runs.
	if tlsx.EnvPath() == "" && requireTaskLocalConfigRoot() == nil {
		cfg, err := cli.LoadCLIConfigForProfile(resolveProfile(cmd))
		if err == nil && strings.TrimSpace(cfg.RootCAFile) != "" {
			if setErr := os.Setenv(tlsx.RootCAEnv, cfg.RootCAFile); setErr != nil {
				fmt.Fprintf(os.Stderr, "warning: could not apply root CA configuration: %v\n", setErr)
				return
			}
		}
	}
	if err := tlsx.InstallFromEnv(); err != nil {
		fmt.Fprintf(os.Stderr, "warning: %v\nrun 'config set root_ca_file \"\"' to clear the configured certificate, then 'connect --root-ca <path>' again.\n", err)
	}
}

func main() {
	envcompat.Bridge()
	if len(os.Args) == 2 && os.Args[1] == execenv.PreparationHelperArg {
		logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
		if err := execenv.RunPreparationHelper(os.Stdin, os.Stdout, logger); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}
	cli.CleanupStaleUpdateArtifacts()
	if err := rootCmd.Execute(); err != nil {
		if err != errSilent {
			fmt.Fprintln(os.Stderr, cli.FormatError(err, debugFlag))
		}
		os.Exit(cli.ExitCodeFor(err))
	}
}
