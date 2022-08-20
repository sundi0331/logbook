package app

import (
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"context"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	apiWatch "k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/watch"
	"k8s.io/client-go/tools/cache"

	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"

	"github.com/sundi0331/logbook/config"
)

func RunRoot(cfg *config.Config, logFile *os.File) {
	handleSingnals(logFile)

	clientset, err := createClientset(&cfg.Auth)
	if err != nil {
		log.Panicln(errors.Wrap(err, "Error occured while creating clientset"))
	}

	watchEvents(cfg, clientset)
}

func handleSingnals(logFile *os.File) {
	sigChan := make(chan os.Signal, 1)
	signal.Ignore()
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)
	go func(logFile *os.File) {
		log.Infoln(logFile)
		for {
			select {
			case sig := <-sigChan:
				switch sig {
				case syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT:
					log.Infof("Signal %v received. Logbook will be shutdown.\n", sig)
					if logFile != nil {
						if err := logFile.Sync(); err != nil {
							fmt.Println(err)
						}
						if err := logFile.Close(); err != nil {
							fmt.Println(err)
						}
					}
					os.Exit(0)
				}
			}
		}
	}(logFile)
}

func watchEvents(cfg *config.Config, clientset *kubernetes.Clientset) {
	// create event watcher
	watchFunc := func(options metav1.ListOptions) (apiWatch.Interface, error) {
		return clientset.CoreV1().Events(cfg.Target.Namespace).Watch(context.TODO(), cfg.Target.ListOptions)
	}
	// eventWatcher, err := clientset.CoreV1().Events(cfg.Target.Namespace).Watch(context.TODO(), cfg.Target.ListOptions)
	eventWatcher, err := watch.NewRetryWatcher("1", &cache.ListWatch{WatchFunc: watchFunc})
	if err != nil {
		log.Panicln(errors.Wrap(err, "Error occured while initializing event watcher"))
	}
	log.Infoln("Event watcher was successfully created. Kubernetes events will be logged from now on.")
	for {
		select {
		case event, ok := <-eventWatcher.ResultChan():
			if !ok {
				log.Warnln("Event watcher result channel closed.")
				os.Exit(0)
			}
			// switch event.Type {
			// case apiWatch.Modified, apiWatch.Added, apiWatch.Error:
			marshalledEvent, err := json.Marshal(event)
			if err != nil {
				log.Panicln(errors.Wrap(err, "Error occured while marshalling event"))
			}
			log.Infof("%s\n", string(marshalledEvent))
			// }
		}
	}
}

func createClientset(authCfg *config.AuthConfig) (*kubernetes.Clientset, error) {
	var config *rest.Config
	var err error

	switch authCfg.Mode {
	case "in-cluster":
		log.Infoln("Logbook will start in in-cluster mode.")
		config, err = rest.InClusterConfig()
		if err != nil {
			return nil, errors.Wrap(err, "Error occured while starting as in-cluster mode")
		}
	case "out-of-cluster":
		log.Infoln("Logbook will start in out-of-cluster mode.")
		if authCfg.KubeConfig == "" {
			log.Infoln("kubeconfig not provided. Will use kubeconfig file in default path.")
			if home := homeDir(); home != "" {
				authCfg.KubeConfig = filepath.Join(home, ".kube", "config")
			}
		}
		// use the current context in kubeconfig
		config, err = clientcmd.BuildConfigFromFlags("", authCfg.KubeConfig)
		if err != nil {
			panic(err.Error())
		}
	default:
		err := fmt.Errorf("auth.mode \"%v\" not supported. Logbook will be terminated.\n", authCfg.Mode)
		return nil, errors.Wrap(err, "Error occured while creating clientset")
	}

	return kubernetes.NewForConfig(config)
}

func homeDir() string {
	if h := os.Getenv("HOME"); h != "" {
		return h
	}
	return os.Getenv("USERPROFILE") // windows
}
