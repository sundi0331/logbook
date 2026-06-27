package config

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

type LogConfig struct {
	Format   string
	Out      string
	Filename string
	Level    string
}

type TargetConfig struct {
	Namespace   string
	Type        string
	ListOptions metav1.ListOptions
}

type AuthConfig struct {
	Mode       string
	KubeConfig string
}

type CheckpointConfig struct {
	Enabled                  bool
	Backend                  string
	Namespace                string
	Name                     string
	Path                     string
	FlushInterval            string `mapstructure:"flush_interval"`
	OnExpiredResourceVersion string `mapstructure:"on_expired_resource_version"`
}

type LeaderElectionConfig struct {
	Enabled       bool
	Namespace     string
	Name          string
	Identity      string
	LeaseDuration string `mapstructure:"lease_duration"`
	RenewDeadline string `mapstructure:"renew_deadline"`
	RetryPeriod   string `mapstructure:"retry_period"`
}

type Config struct {
	Log            LogConfig
	Target         TargetConfig
	Auth           AuthConfig
	Checkpoint     CheckpointConfig
	LeaderElection LeaderElectionConfig `mapstructure:"leader_election"`
}
