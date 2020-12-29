package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	flag "github.com/spf13/pflag"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/strategicpatch"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

type labels map[string]string

const (
	logLevelAll   = "all"
	logLevelDebug = "debug"
	logLevelInfo  = "info"
	logLevelWarn  = "warn"
	logLevelError = "error"
	logLevelNone  = "none"
)

var (
	kubeconfig         = flag.String("kubeconfig", "", "path to kubeconfig")
	hostname           = flag.String("hostname", "", "Hostname of the node on which this process is running")
	lMod               = flag.StringSliceP("label-mod", "m", []string{}, "list of strings, kernel modules matching a string will be used as labels with values true, if found")
	logLevel           = flag.String("log-level", logLevelInfo, fmt.Sprintf("Log level to use. Possible values: %s", availableLogLevels))
	updateTime         = flag.Duration("update-time", 10*time.Second, "renewal time for labels in seconds")
	noCleanUp          = flag.Bool("no-clean-up", false, "Will not attempt to clean labels before shutting down")
	labelPrefix        = flag.String("label-prefix", "nkml.squat.ai", "prefix for labels")
	addr               = flag.String("listen-address", ":8080", "listen address for prometheus metrics server")
	availableLogLevels = strings.Join([]string{
		logLevelAll,
		logLevelDebug,
		logLevelInfo,
		logLevelWarn,
		logLevelError,
		logLevelNone,
	}, ", ")
)

var (
	reconcilingCounter = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "reconciling_counter",
			Help: "Number of reconciling outcomes",
		},
		[]string{"success"},
	)
)

// scanMod scans modules in /proc/modules and returns lables with prefix
func scanMod() (labels, error) {
	file, err := os.Open("/proc/modules")
	if err != nil {
		return nil, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)

	mods := make(map[string]struct{})
	for scanner.Scan() {
		s := strings.Split(scanner.Text(), " ")
		mods[s[0]] = struct{}{}
	}
	ret := make(labels)
	for _, m := range *lMod {
		if _, ok := mods[m]; ok {
			ret[fmt.Sprintf("%s/%s", *labelPrefix, m)] = "true"
		} else {
			ret[fmt.Sprintf("%s/%s", *labelPrefix, m)] = "false"
		}
	}
	return ret, nil
}

// filter filters keys in map of strings by their prefix and returns the filtered labels
func filter(m map[string]string) labels {
	ret := make(labels)
	for k, v := range m {
		if strings.HasPrefix(k, *labelPrefix) {
			ret[k] = v
		}
	}
	return ret
}

// merge merges labels into a map of strings
// and returnes a map, after deleting the keys
// that start with the prefix labelPrefix
func merge(l map[string]string, ul labels) map[string]string {
	// delete old labels
	for k := range filter(l) {
		if _, e := ul[k]; !e {
			delete(l, k)
		}
	}
	// add new labels to map
	for k, v := range ul {
		l[k] = v
	}
	return l
}

// getNode returns the node with name hostname or an error
func getNode(ctx context.Context, clientset *kubernetes.Clientset) (*v1.Node, error) {
	node, err := clientset.CoreV1().Nodes().Get(ctx, *hostname, metav1.GetOptions{})
	if errors.IsNotFound(err) {
		return nil, fmt.Errorf("node not found: %w", err)
	} else if err != nil {
		return nil, fmt.Errorf("could not get node: %w", err)
	}
	return node, nil
}

// scanAndLabel scans and labels the node with name hostname or returns an error
func scanAndLabel(ctx context.Context, clientset *kubernetes.Clientset, logger log.Logger) error {
	node, err := getNode(ctx, clientset)
	if err != nil {
		return err
	}
	oldData, err := json.Marshal(node)
	if err != nil {
		return err
	}
	// scan modules
	l, err := scanMod()
	if err != nil {
		return fmt.Errorf("could not scan modules: %w", err)
	}
	node.ObjectMeta.Labels = merge(node.ObjectMeta.Labels, l)
	newData, err := json.Marshal(node)
	if err != nil {
		return fmt.Errorf("failed to marshal labels: %w", err)
	}
	patch, err := strategicpatch.CreateTwoWayMergePatch(oldData, newData, v1.Node{})
	if err != nil {
		return fmt.Errorf("failed to create patch for node %q: %w", node.Name, err)
	}
	if nn, err := clientset.CoreV1().Nodes().Patch(ctx, node.Name, types.StrategicMergePatchType, patch, metav1.PatchOptions{}); err != nil {
		return fmt.Errorf("failed to patch node: %w", err)
	} else {
		level.Debug(logger).Log("msg", fmt.Sprintf("patched labels: %v", nn.ObjectMeta.Labels))
	}
	return nil
}

// cleanUp will remove all labels with prefix labelPrefix from the node with name hostname or return an error
func cleanUp(clientset *kubernetes.Clientset, logger log.Logger) error {
	ctx := context.Background()
	node, err := getNode(ctx, clientset)
	if err != nil {
		return err
	}
	oldData, err := json.Marshal(node)
	if err != nil {
		return err
	}
	for k := range node.ObjectMeta.Labels {
		if strings.HasPrefix(k, *labelPrefix) {
			delete(node.ObjectMeta.Labels, k)
		}
	}
	newData, err := json.Marshal(node)
	if err != nil {
		return err
	}

	patch, err := strategicpatch.CreateTwoWayMergePatch(oldData, newData, v1.Node{})
	if err != nil {
		return fmt.Errorf("failed to create patch: %w", err)
	}
	if nn, err := clientset.CoreV1().Nodes().Patch(ctx, node.Name, types.StrategicMergePatchType, patch, metav1.PatchOptions{}); err != nil {
		return fmt.Errorf("could not patch node: %w", err)
	} else {
		level.Info(logger).Log("msg", "successfully cleaned node")
		level.Debug(logger).Log("msg", fmt.Sprintf("labels of cleaned node: %v", nn.ObjectMeta.Labels))
	}
	return nil
}

func Main() error {
	flag.Parse()

	logger := log.NewJSONLogger(log.NewSyncWriter(os.Stdout))
	switch *logLevel {
	case logLevelAll:
		logger = level.NewFilter(logger, level.AllowAll())
	case logLevelDebug:
		logger = level.NewFilter(logger, level.AllowDebug())
	case logLevelInfo:
		logger = level.NewFilter(logger, level.AllowInfo())
	case logLevelWarn:
		logger = level.NewFilter(logger, level.AllowWarn())
	case logLevelError:
		logger = level.NewFilter(logger, level.AllowError())
	case logLevelNone:
		logger = level.NewFilter(logger, level.AllowNone())
	default:
		return fmt.Errorf("log level %v unknown; possible values are: %s", *logLevel, availableLogLevels)
	}
	logger = log.With(logger, "ts", log.DefaultTimestampUTC)
	logger = log.With(logger, "caller", log.DefaultCaller)

	// create context to be able to cancel calls to the kubernetes API in clean up
	ctx, cancel := context.WithCancel(context.Background())

	// create prometheus registry to not use default one
	r := prometheus.NewRegistry()
	r.MustRegister(
		reconcilingCounter,
		prometheus.NewGoCollector(),
		prometheus.NewProcessCollector(prometheus.ProcessCollectorOpts{}),
	)
	m := http.NewServeMux()
	m.Handle("/metrics", promhttp.HandlerFor(r, promhttp.HandlerOpts{}))
	// create global var for server to be able to stop the server later
	msrv := &http.Server{
		Addr:    *addr,
		Handler: m,
	}
	go func() {
		level.Info(logger).Log("msg", "starting metrics server")
		if err := msrv.ListenAndServe(); err != nil {
			level.Error(logger).Log("msg", "could not start metrics server", "err", err)
		}
	}()

	// generate kubeconfig
	var config *rest.Config
	var err error
	if *kubeconfig == "" {
		config, err = rest.InClusterConfig()
		if err == rest.ErrNotInCluster {
			return fmt.Errorf("not in cluster: %w", err)
		} else if err != nil {
			return err
		}
		level.Info(logger).Log("msg", "generated in cluster config")
	} else {
		config, err = clientcmd.BuildConfigFromFlags("", *kubeconfig)
		if err != nil {
			return fmt.Errorf("could not generate kubernetes config: %w", err)
		}
		level.Info(logger).Log("msg", fmt.Sprintf("generated config with kubeconfig: %s", *kubeconfig))
	}
	// create the clientset
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return fmt.Errorf("could not generate clientset: %w", err)
	}

	ch := make(chan os.Signal, 1)
	signal.Notify(ch, os.Interrupt, syscall.SIGTERM, syscall.SIGQUIT)

	level.Info(logger).Log("msg", "start service", "label-prefix", *labelPrefix, "label-mod", *lMod)
	// use a mutex to avoid simultaneous updates at small update-time or slow network speed
	var mutex sync.Mutex
	for {
		select {
		case s := <-ch:
			level.Info(logger).Log("msg", fmt.Sprintf("received signal %v", s))
			// cancel context for running scan and label routine
			cancel()
			// lock mutex to wait until running scan and label routine is finished
			mutex.Lock()
			if *noCleanUp == false {
				if err := cleanUp(clientset, logger); err != nil {
					level.Error(logger).Log("msg", "could not clean node", "err", err)
				}
			}
			if err := msrv.Close(); err != nil {
				level.Error(logger).Log("msg", "could not close metrics server", "err", err)
			} else {
				level.Info(logger).Log("msg", "closing metrics server")
			}
			level.Info(logger).Log("msg", "shutting down")
			os.Exit(130)
		case <-time.After(*updateTime):
			mutex.Lock()
			// use a go routine, so the time to update the labels doesn't influence the frequency of updates
			go func() {
				defer mutex.Unlock()
				if err := scanAndLabel(ctx, clientset, logger); err != nil {
					level.Error(logger).Log("msg", "failed to scan and label", "err", err)
					reconcilingCounter.With(prometheus.Labels{"success": "false"}).Inc()
				} else {
					reconcilingCounter.With(prometheus.Labels{"success": "true"}).Inc()
				}
			}()
		}
	}
}

func main() {
	if err := Main(); err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
}
