package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/rtfpessoa/clitr/internal/log"
	"github.com/rtfpessoa/clitr/internal/utils"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

const (
	verboseFlagKeyShort = "v"
	verboseFlagKeyLong  = "verbose"

	dataDirFlagKeyShort = "d"
	dataDirFlagKeyLong  = "data-dir"
	defaultDataDir      = "~/.local/share/clitr"
)

type rootConfig struct {
	dataDir   string
	eventsDir string
	verbose   bool
}

func main() {
	config := &rootConfig{}

	defer log.Sync()

	rootCmd := &cobra.Command{
		Use:   "clitr",
		Short: "Trade Republic transaction exporter",
		Long:  "A CLI tool to fetch and export transaction history from Trade Republic",
	}

	rootCmd.PersistentFlags().BoolVarP(&config.verbose, verboseFlagKeyLong, verboseFlagKeyShort, false, "Enable verbose debug output")
	rootCmd.PersistentFlags().StringVarP(&config.dataDir, dataDirFlagKeyLong, dataDirFlagKeyShort, defaultDataDir, fmt.Sprintf("Directory to save data (default: %s)", defaultDataDir))

	// This runs before any subcommands
	rootCmd.PersistentPreRunE = func(cmd *cobra.Command, args []string) error {
		if config.verbose {
			log.SetLevel(zapcore.DebugLevel)
		}

		if config.dataDir == "" {
			return fmt.Errorf("--%s cannot be empty, to use the default value do not pass the flag", dataDirFlagKeyLong)
		}

		resolvedDataDir, err := utils.ResolvePath(config.dataDir)
		if err != nil {
			return fmt.Errorf("failed to resolve %s: %w", dataDirFlagKeyLong, err)
		}

		config.dataDir = resolvedDataDir
		config.eventsDir = filepath.Join(config.dataDir, "events")

		return nil
	}

	rootCmd.AddCommand(
		NewFetchCmd(config),
		NewExportCmd(config),
		NewPatchCmd(config),
	)

	if err := rootCmd.Execute(); err != nil {
		log.Error("Command failed", zap.Error(err))
		os.Exit(1)
	}
}
