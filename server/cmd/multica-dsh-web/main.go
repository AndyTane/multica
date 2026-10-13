package main

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/multica-ai/multica/server/internal/dshweb"
)

func main() {
	node := os.Getenv("MULTICA_DSH_NODE_PATH")
	if node == "" {
		node = "node"
	}
	args := []string{"--input-type=module", "--eval", dshweb.Script + "\ntry { await main(process.argv.slice(1)); } catch (error) { process.stderr.write(reasonOf(error) + '\\n'); process.exitCode = 1; }", "--"}
	cmd := exec.Command(node, append(args, os.Args[1:]...)...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		if exit, ok := err.(*exec.ExitError); ok {
			os.Exit(exit.ExitCode())
		}
		fmt.Fprintln(os.Stderr, "DSH web bridge requires Node.js 24 or newer")
		os.Exit(1)
	}
}
