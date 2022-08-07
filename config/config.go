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

type Config struct {
	Log    LogConfig
	Target TargetConfig
	Auth   AuthConfig
}
