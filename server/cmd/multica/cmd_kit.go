package main

import (
	"context"

	"github.com/multica-ai/multica/server/internal/managedkit"
	"github.com/spf13/cobra"
)

func init() {
	var root string
	cmd := &cobra.Command{Use: "kit-recover", Hidden: true, RunE: func(cmd *cobra.Command, args []string) error {
		if err := requireHumanLocalCommand("kit-recover"); err != nil {
			return err
		}
		return managedkit.RunHelper(context.Background(), root)
	}}
	cmd.Flags().StringVar(&root, "root", "", "Managed installation root")
	_ = cmd.MarkFlagRequired("root")
	rootCmd.AddCommand(cmd)
}
