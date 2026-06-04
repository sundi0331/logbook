package cmd

import (
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/sundi0331/logbook/app"
	"github.com/sundi0331/logbook/config"
	internallogging "github.com/sundi0331/logbook/internal/logging"
)

var cfgFile string
var cfg config.Config

var rootCmd = &cobra.Command{
	Use:   "logbook",
	Short: "Logbook is a Kubernetes event logger.",
	Long: `Logbook is a Kubernetes event logger which can run either
in-cluster using a Kubernetes ServiceAccount or out-of-cluster using a kubeconfig file.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		logger, cleanup, err := initialize()
		if err != nil {
			return err
		}
		defer func() {
			if err := cleanup(); err != nil {
				logger.Error("failed to close logger", "error", err)
			}
		}()

		return app.Run(cmd.Context(), &cfg, logger)
	},
}

func Execute() error {
	return rootCmd.Execute()
}

func init() {
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is $PWD/logbook.yaml)")
	rootCmd.PersistentFlags().StringVar(&cfg.Auth.Mode, "mode", "", "running mode (default is in-cluster mode)")
	rootCmd.PersistentFlags().StringVar(&cfg.Auth.KubeConfig, "kubeconfig", "", "absolute path of kubeconfig file (default is $HOME/.kube/config, only used in out-of-cluster mode)")
	rootCmd.PersistentFlags().StringVar(&cfg.Target.Namespace, "namespace", "", "namespace to watch (default is all namespaces)")
	rootCmd.PersistentFlags().StringVar(&cfg.Log.Format, "log-format", "", "log format: json or text (default is json)")
	rootCmd.PersistentFlags().StringVar(&cfg.Log.Out, "log-out", "", "log output: stdout, stderr, or file (default is stdout)")
	rootCmd.PersistentFlags().StringVar(&cfg.Log.Level, "log-level", "", "log level: debug, info, warn, or error (default is info)")
	rootCmd.PersistentFlags().StringVar(&cfg.Log.Filename, "log-filename", "", "full path of log file with filename (valid only when log-out is set to file; default is k8s-events.log)")

	mustBindPFlag("config", "config")
	mustBindPFlag("auth.mode", "mode")
	mustBindPFlag("auth.kubeconfig", "kubeconfig")
	mustBindPFlag("target.namespace", "namespace")
	mustBindPFlag("log.format", "log-format")
	mustBindPFlag("log.out", "log-out")
	mustBindPFlag("log.level", "log-level")
	mustBindPFlag("log.filename", "log-filename")
}

func mustBindPFlag(key, flag string) {
	if err := viper.BindPFlag(key, rootCmd.PersistentFlags().Lookup(flag)); err != nil {
		panic(fmt.Sprintf("bind flag %q: %v", flag, err))
	}
}

func initialize() (*slog.Logger, func() error, error) {
	setDefaults()
	configureFileLookup()

	viper.SetEnvPrefix("logbook")
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	viper.AutomaticEnv()

	if err := viper.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, nil, fmt.Errorf("read config: %w", err)
		}
	}

	if err := viper.Unmarshal(&cfg); err != nil {
		return nil, nil, fmt.Errorf("unmarshal config: %w", err)
	}

	logger, cleanup, err := internallogging.New(cfg.Log)
	if err != nil {
		return nil, nil, fmt.Errorf("initialize logger: %w", err)
	}
	slog.SetDefault(logger)

	if viper.ConfigFileUsed() != "" {
		logger.Info("using config file", "path", viper.ConfigFileUsed())
	}
	logger.Info("initialized configuration", "auth_mode", cfg.Auth.Mode, "target_namespace", cfg.Target.Namespace, "log_format", cfg.Log.Format, "log_output", cfg.Log.Out, "log_level", cfg.Log.Level)

	return logger, cleanup, nil
}

func setDefaults() {
	viper.SetDefault("target.namespace", "")
	viper.SetDefault("log.format", "json")
	viper.SetDefault("log.out", "stdout")
	viper.SetDefault("log.filename", "k8s-events.log")
	viper.SetDefault("log.level", "info")
	viper.SetDefault("auth.mode", "in-cluster")
}

func configureFileLookup() {
	if cfgFile != "" {
		viper.SetConfigFile(cfgFile)
		return
	}

	home, err := os.UserHomeDir()
	if err == nil && home != "" {
		viper.AddConfigPath(home + "/.logbook")
	}
	viper.SetConfigType("yaml")
	viper.SetConfigName("logbook")
	viper.AddConfigPath(".")
	viper.AddConfigPath("/etc/logbook")
}
