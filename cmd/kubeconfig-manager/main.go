package main

import (
	"context"
	"os"

	"github.com/charmbracelet/fang"

	"github.com/loupeznik/kubeconfig-manager/internal/cli"
)

func main() {
	err := fang.Execute(
		context.Background(),
		cli.NewRootCmd(),
		fang.WithVersion(cli.Version),
		fang.WithCommit(cli.Commit),
	)
	if err != nil {
		os.Exit(1)
	}
}
