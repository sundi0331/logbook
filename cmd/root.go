/*
Copyright © 2022 sundi0331@gmail.com

*/
package cmd

import (
	"os"
	"strings"

	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/sundi0331/logbook/config"
	"github.com/sundi0331/logbook/app"
)

var cfgFile string
var cfg config.Config
var logFile *os.File

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "logbook",
	Short: "Logbook is a kubernetes event logger.",
	Long: `Logbook is a kubernetes event logger which can be used
both in-cluster(use kubernetes ServiceAccount for auth) 
and out-of-cluster(use kubeconfig file for auth).`,
	// Uncomment the following line if your bare application
	// has an action associated with it:
	Run: func(cmd *cobra.Command, args []string) {
		app.RunRoot(&cfg, logFile)
	},
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	err := rootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}

func init() {
	cobra.OnInitialize(initConfig)

	// Here you will define your flags and configuration settings.
	// Cobra supports persistent flags, which, if defined here,
	// will be global for your application.

	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is $PWD/logbook.yaml)")
	rootCmd.PersistentFlags().StringVar(&cfg.Auth.Mode, "mode", "", "running mode (default is in-cluster mode)")
	rootCmd.PersistentFlags().StringVar(&cfg.Auth.KubeConfig, "kubeconfig", "", "absolute path of kubeconfig file (default is $HOME/.kube/config, only used in out-of-cluster mode)")
	rootCmd.PersistentFlags().StringVar(&cfg.Target.Namespace, "namespace", "", "namespace to watch (default is all namespaces)")
	rootCmd.PersistentFlags().StringVar(&cfg.Log.Format, "log-format", "", "log format (default is json)")
	rootCmd.PersistentFlags().StringVar(&cfg.Log.Out, "log-out", "", "log output (default is stdout)")
	rootCmd.PersistentFlags().StringVar(&cfg.Log.Level, "log-level", "", "log level (default is info)")
	rootCmd.PersistentFlags().StringVar(&cfg.Log.Filename, "log-filename", "", "full path of log file with filename (valid only when log-out is set to file. default is k8s-events.log in the same directory as logbook)")

	// Cobra also supports local flags, which will only run
	// when this action is called directly.
	// rootCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
	viper.BindPFlag("config", rootCmd.PersistentFlags().Lookup("config"))
	viper.BindPFlag("auth.mode", rootCmd.PersistentFlags().Lookup("mode"))
	viper.BindPFlag("auth.kubeconfig", rootCmd.PersistentFlags().Lookup("kubeconfig"))
	viper.BindPFlag("target.namespace", rootCmd.PersistentFlags().Lookup("namespace"))
	viper.BindPFlag("log.format", rootCmd.PersistentFlags().Lookup("log-format"))
	viper.BindPFlag("log.out", rootCmd.PersistentFlags().Lookup("log-out"))
	viper.BindPFlag("log.level", rootCmd.PersistentFlags().Lookup("log-level"))
	viper.BindPFlag("log.filename", rootCmd.PersistentFlags().Lookup("log-filename"))
}

// initConfig reads in config file and ENV variables if set.
func initConfig() {
	// default values
	viper.SetDefault("target.namespace", "")
	timeoutSeconds := int64(315360000)  // 10 years
	viper.SetDefault("target.listoptions.timeoutseconds", &timeoutSeconds)
	viper.SetDefault("log.format", "json")
	viper.SetDefault("log.out", "stdout")
	viper.SetDefault("log.filename", "k8s-events.log")
	viper.SetDefault("log.level", "info")
	viper.SetDefault("auth.mode", "in-cluster")

	if viper.Get("config") != nil && viper.Get("config").(string) != "" {
		viper.SetConfigFile(viper.Get("config").(string))
	} else {
		// Find home directory.
		home, err := os.UserHomeDir()
		cobra.CheckErr(err)

		viper.SetConfigType("yaml")
		viper.SetConfigName("logbook")
		viper.AddConfigPath(".")
		viper.AddConfigPath(home + "/.logbook")
		viper.AddConfigPath("/etc/logbook")
	}

	viper.AutomaticEnv()
	viper.SetEnvPrefix("logbook")
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))

	// If a config file is found, read it in.
	if err := viper.ReadInConfig(); err == nil {
		log.Infoln("Using config file:", viper.ConfigFileUsed())
	} else {
		log.Infoln("Error opening config file:", err.Error())
	}

	if err := viper.Unmarshal(&cfg); err != nil {
		log.Panicln(errors.Wrap(err, "Error occured during unmarshal config"))
	}
	log.Infof("Initialized with configuration: %+v\n", cfg)

	//init logrus
	var err error
	if logFile, err = initLogrus(&cfg.Log); err != nil {
		log.Panicln(errors.Wrap(err, "Error occured during logrus initialization"))
	}
}

func initLogrus(logCfg *config.LogConfig) (*os.File, error) {
	var file *os.File
	switch logCfg.Format {
	case "json":
		log.SetFormatter(&log.JSONFormatter{})
	case "text":
		log.SetFormatter(&log.TextFormatter{})
	default:
		log.Errorf("log.format \"%v\" not supported, defaults to json.\n", logCfg.Format)
		log.SetFormatter(&log.JSONFormatter{})
	}

	switch logCfg.Out {
	case "stdout":
		log.SetOutput(os.Stdout)
	case "stderr":
		log.SetOutput(os.Stderr)
	case "file":
		f, err := os.OpenFile(logCfg.Filename, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			return nil, errors.Wrap(err, "Error occured during open log file")
		}
		log.SetOutput(f)
		file = f
	default:
		log.Errorf("log.out \"%v\" not supported, defaults to stdout.", logCfg.Out)
		log.SetOutput(os.Stdout)
	}

	switch logCfg.Level {
	case "panic":
		log.SetLevel(log.PanicLevel)
	case "fatal":
		log.SetLevel(log.FatalLevel)
	case "error":
		log.SetLevel(log.ErrorLevel)
	case "warn":
		log.SetLevel(log.WarnLevel)
	case "info":
		log.SetLevel(log.InfoLevel)
	case "debug":
		log.SetLevel(log.DebugLevel)
	case "trace":
		log.SetLevel(log.TraceLevel)
	default:
		log.Errorf("log.level \"%v\" not supported, defaults to info.", logCfg.Level)
		log.SetLevel(log.InfoLevel)
	}

	return file, nil
}
