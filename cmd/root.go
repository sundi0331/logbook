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
	rootCmd.PersistentFlags().BoolVar(&cfg.Checkpoint.Enabled, "checkpoint-enabled", true, "enable Kubernetes event resource version checkpointing (default is true)")
	rootCmd.PersistentFlags().StringVar(&cfg.Checkpoint.Backend, "checkpoint-backend", "", "checkpoint backend: configmap or file (default is configmap)")
	rootCmd.PersistentFlags().StringVar(&cfg.Checkpoint.Namespace, "checkpoint-namespace", "", "namespace for the checkpoint ConfigMap (default is POD_NAMESPACE or default)")
	rootCmd.PersistentFlags().StringVar(&cfg.Checkpoint.Name, "checkpoint-name", "", "name of the checkpoint ConfigMap (default is logbook-checkpoint)")
	rootCmd.PersistentFlags().StringVar(&cfg.Checkpoint.Path, "checkpoint-path", "", "path for file checkpoint backend (default is logbook-checkpoint)")
	rootCmd.PersistentFlags().StringVar(&cfg.Checkpoint.FlushInterval, "checkpoint-flush-interval", "", "checkpoint flush interval; set to 0s to flush every event (default is 5s)")
	rootCmd.PersistentFlags().StringVar(&cfg.Checkpoint.OnExpiredResourceVersion, "checkpoint-on-expired-resource-version", "", "behavior when a checkpoint resource version expires: skip-existing or fail (default is skip-existing)")
	rootCmd.PersistentFlags().BoolVar(&cfg.LeaderElection.Enabled, "leader-election-enabled", false, "enable Kubernetes Lease leader election (default is false)")
	rootCmd.PersistentFlags().StringVar(&cfg.LeaderElection.Namespace, "leader-election-namespace", "", "namespace for the leader election Lease (default is POD_NAMESPACE or default)")
	rootCmd.PersistentFlags().StringVar(&cfg.LeaderElection.Name, "leader-election-name", "", "name of the leader election Lease (default is logbook-leader)")
	rootCmd.PersistentFlags().StringVar(&cfg.LeaderElection.Identity, "leader-election-identity", "", "identity for this leader election candidate (default is POD_NAME or hostname)")
	rootCmd.PersistentFlags().StringVar(&cfg.LeaderElection.LeaseDuration, "leader-election-lease-duration", "", "leader election lease duration (default is 15s)")
	rootCmd.PersistentFlags().StringVar(&cfg.LeaderElection.RenewDeadline, "leader-election-renew-deadline", "", "leader election renew deadline (default is 10s)")
	rootCmd.PersistentFlags().StringVar(&cfg.LeaderElection.RetryPeriod, "leader-election-retry-period", "", "leader election retry period (default is 2s)")

	mustBindPFlag("config", "config")
	mustBindPFlag("auth.mode", "mode")
	mustBindPFlag("auth.kubeconfig", "kubeconfig")
	mustBindPFlag("target.namespace", "namespace")
	mustBindPFlag("log.format", "log-format")
	mustBindPFlag("log.out", "log-out")
	mustBindPFlag("log.level", "log-level")
	mustBindPFlag("log.filename", "log-filename")
	mustBindPFlag("checkpoint.enabled", "checkpoint-enabled")
	mustBindPFlag("checkpoint.backend", "checkpoint-backend")
	mustBindPFlag("checkpoint.namespace", "checkpoint-namespace")
	mustBindPFlag("checkpoint.name", "checkpoint-name")
	mustBindPFlag("checkpoint.path", "checkpoint-path")
	mustBindPFlag("checkpoint.flush_interval", "checkpoint-flush-interval")
	mustBindPFlag("checkpoint.on_expired_resource_version", "checkpoint-on-expired-resource-version")
	mustBindPFlag("leader_election.enabled", "leader-election-enabled")
	mustBindPFlag("leader_election.namespace", "leader-election-namespace")
	mustBindPFlag("leader_election.name", "leader-election-name")
	mustBindPFlag("leader_election.identity", "leader-election-identity")
	mustBindPFlag("leader_election.lease_duration", "leader-election-lease-duration")
	mustBindPFlag("leader_election.renew_deadline", "leader-election-renew-deadline")
	mustBindPFlag("leader_election.retry_period", "leader-election-retry-period")
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
	logger.Info("initialized configuration", "auth_mode", cfg.Auth.Mode, "target_namespace", cfg.Target.Namespace, "log_format", cfg.Log.Format, "log_output", cfg.Log.Out, "log_level", cfg.Log.Level, "checkpoint_enabled", cfg.Checkpoint.Enabled, "checkpoint_backend", cfg.Checkpoint.Backend, "checkpoint_namespace", cfg.Checkpoint.Namespace, "checkpoint_flush_interval", cfg.Checkpoint.FlushInterval, "leader_election_enabled", cfg.LeaderElection.Enabled, "leader_election_namespace", cfg.LeaderElection.Namespace, "leader_election_name", cfg.LeaderElection.Name)

	return logger, cleanup, nil
}

func setDefaults() {
	viper.SetDefault("target.namespace", "")
	viper.SetDefault("log.format", "json")
	viper.SetDefault("log.out", "stdout")
	viper.SetDefault("log.filename", "k8s-events.log")
	viper.SetDefault("log.level", "info")
	viper.SetDefault("auth.mode", "in-cluster")
	viper.SetDefault("checkpoint.enabled", true)
	viper.SetDefault("checkpoint.backend", "configmap")
	viper.SetDefault("checkpoint.namespace", "")
	viper.SetDefault("checkpoint.name", "logbook-checkpoint")
	viper.SetDefault("checkpoint.path", "logbook-checkpoint")
	viper.SetDefault("checkpoint.flush_interval", "5s")
	viper.SetDefault("checkpoint.on_expired_resource_version", "skip-existing")
	viper.SetDefault("leader_election.enabled", false)
	viper.SetDefault("leader_election.namespace", "")
	viper.SetDefault("leader_election.name", "logbook-leader")
	viper.SetDefault("leader_election.identity", "")
	viper.SetDefault("leader_election.lease_duration", "15s")
	viper.SetDefault("leader_election.renew_deadline", "10s")
	viper.SetDefault("leader_election.retry_period", "2s")
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
