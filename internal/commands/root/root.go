// Copyright © 2017 The virtual-kubelet authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package root

import (
	"context"
	"os"
	"path"
	"time"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"github.com/virtual-kubelet/node-cli/manager"
	"github.com/virtual-kubelet/node-cli/opts"
	"github.com/virtual-kubelet/node-cli/provider"
	"github.com/virtual-kubelet/virtual-kubelet/errdefs"
	"github.com/virtual-kubelet/virtual-kubelet/log"
	"github.com/virtual-kubelet/virtual-kubelet/node"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/types"
	kubeinformers "k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	v1 "k8s.io/client-go/kubernetes/typed/coordination/v1"
	corev1client "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/record"
)

// NewCommand creates a new top-level command.
// This command is used to start the virtual-kubelet daemon
func NewCommand(name string, s *provider.Store, o *opts.Opts) *cobra.Command {
	cmd := &cobra.Command{
		Use:   name,
		Short: name + " provides a virtual kubelet interface for your kubernetes cluster.",
		Long: name + ` implements the Kubelet interface with a pluggable
backend implementation allowing users to create kubernetes nodes without running the kubelet.
This allows users to schedule kubernetes workloads on nodes that aren't running Kubernetes.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runRootCommand(cmd.Context(), s, o)
		},
	}

	applyDefaults(o)
	installFlags(cmd.Flags(), o)

	return cmd
}

func applyDefaults(o *opts.Opts) {
	o.Authentication.Webhook.Enabled = false
}

func runRootCommand(ctx context.Context, s *provider.Store, c *opts.Opts) error {
	var err error

	pInit := s.Get(c.Provider)
	if pInit == nil {
		return errors.Errorf("provider %q not found", c.Provider)
	}

	client := c.KubernetesClient
	if client == nil {
		client, err = newClient(c.KubeConfigPath, c.KubeAPIQPS, c.KubeAPIBurst)
		if err != nil {
			return err
		}
	}

	return runRootCommandWithProviderAndClient(ctx, pInit, client, c)
}

func runRootCommandWithProviderAndClient(ctx context.Context, pInit provider.InitFunc, client kubernetes.Interface, c *opts.Opts) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	if ok := provider.ValidOperatingSystems[c.OperatingSystem]; !ok {
		return errdefs.InvalidInputf("operating system %q is not supported", c.OperatingSystem)
	}

	if c.PodSyncWorkers == 0 {
		return errdefs.InvalidInput("pod sync workers must be greater than 0")
	}

	var taint *corev1.Taint
	if !c.DisableTaint {
		var err error
		taint, err = getTaint(c)
		if err != nil {
			return err
		}
	}

	// Create a shared informer factory for Kubernetes pods in the current namespace (if specified) and scheduled to the current node.
	podInformerFactory := kubeinformers.NewSharedInformerFactoryWithOptions(
		client,
		c.InformerResyncPeriod,
		kubeinformers.WithNamespace(c.KubeNamespace),
		kubeinformers.WithTweakListOptions(func(options *metav1.ListOptions) {
			options.FieldSelector = fields.OneTermEqualSelector("spec.nodeName", c.NodeName).String()
		}))
	podInformer := podInformerFactory.Core().V1().Pods()

	// Create another shared informer factory for Kubernetes secrets and configmaps (not subject to any selectors).
	scmInformerFactory := kubeinformers.NewSharedInformerFactoryWithOptions(client, c.InformerResyncPeriod)
	// Create a secret informer and a config map informer so we can pass their listers to the resource manager.
	secretInformer := scmInformerFactory.Core().V1().Secrets()
	configMapInformer := scmInformerFactory.Core().V1().ConfigMaps()
	serviceInformer := scmInformerFactory.Core().V1().Services()
	pvcInformer := scmInformerFactory.Core().V1().PersistentVolumeClaims()
	pvInformer := scmInformerFactory.Core().V1().PersistentVolumes()

	rm, err := manager.NewResourceManager(podInformer.Lister(),
		secretInformer.Lister(),
		configMapInformer.Lister(),
		serviceInformer.Lister(),
		pvcInformer.Lister(),
		pvInformer.Lister())
	if err != nil {
		return errors.Wrap(err, "could not create resource manager")
	}

	// Start the informers now, so the provider will get a functional resource
	// manager.
	podInformerFactory.Start(ctx.Done())
	scmInformerFactory.Start(ctx.Done())

	apiConfig, err := getAPIConfig(c)
	if err != nil {
		return err
	}

	if apiConfig.AuthWebhookEnabled {
		// TODO(guwe): handle CA rotate?
		auth, _, err := BuildAuth(types.NodeName(c.NodeName), client, *c)
		if err != nil {
			return err
		}
		apiConfig.Auth = auth
	}

	initConfig := provider.InitConfig{
		ConfigPath:        c.ProviderConfigPath,
		NodeName:          c.NodeName,
		OperatingSystem:   c.OperatingSystem,
		ResourceManager:   rm,
		DaemonPort:        int32(c.ListenPort),
		InternalIP:        os.Getenv("VKUBELET_POD_IP"),
		KubeClusterDomain: c.KubeClusterDomain,
	}

	p, err := pInit(initConfig)
	if err != nil {
		return errors.Wrapf(err, "error initializing provider %s", c.Provider)
	}

	ctx = log.WithLogger(ctx, log.G(ctx).WithFields(log.Fields{
		"provider":         c.Provider,
		"operatingSystem":  c.OperatingSystem,
		"node":             c.NodeName,
		"watchedNamespace": c.KubeNamespace,
	}))

	var leaseClient v1.LeaseInterface
	if c.EnableNodeLease {
		leaseClient = client.CoordinationV1().Leases(corev1.NamespaceNodeLease)
	}

	nodeProvider, ok := p.(node.NodeProvider)
	if !ok {
		nodeProvider = node.NaiveNodeProvider{}
	}
	pNode := NodeFromProvider(ctx, c.NodeName, taint, p, c.Version)
	nodeRunner, err := node.NewNodeController(
		nodeProvider,
		pNode,
		client.CoreV1().Nodes(),
		node.WithNodeEnableLeaseV1(leaseClient, node.DefaultLeaseDuration),
		node.WithNodeStatusUpdateErrorHandler(func(ctx context.Context, err error) error {
			if !k8serrors.IsNotFound(err) {
				return err
			}

			log.G(ctx).Debug("node not found")
			newNode := pNode.DeepCopy()
			newNode.ResourceVersion = ""
			_, err = client.CoreV1().Nodes().Create(ctx, newNode, metav1.CreateOptions{})
			if err != nil {
				return err
			}
			log.G(ctx).Debug("created new node")
			return nil
		}),
	)
	if err != nil {
		log.G(ctx).Fatal(err)
	}

	eb := record.NewBroadcaster()
	eb.StartLogging(log.G(ctx).Infof)
	eb.StartRecordingToSink(&corev1client.EventSinkImpl{Interface: client.CoreV1().Events(c.KubeNamespace)})

	pc, err := node.NewPodController(node.PodControllerConfig{
		PodClient:                            client.CoreV1(),
		PodInformer:                          podInformer,
		EventRecorder:                        eb.NewRecorder(scheme.Scheme, corev1.EventSource{Component: path.Join(pNode.Name, "pod-controller")}),
		Provider:                             p,
		SecretInformer:                       secretInformer,
		ConfigMapInformer:                    configMapInformer,
		ServiceInformer:                      serviceInformer,
		SyncPodsFromKubernetesRateLimiter:    c.SyncPodsFromKubernetesRateLimiter,
		DeletePodsFromKubernetesRateLimiter:  c.DeletePodsFromKubernetesRateLimiter,
		SyncPodStatusFromProviderRateLimiter: c.SyncPodStatusFromProviderRateLimiter,
	})
	if err != nil {
		return errors.Wrap(err, "error setting up pod controller")
	}

	cancelHTTP, err := setupHTTPServer(ctx, p, apiConfig)
	if err != nil {
		return err
	}
	defer cancelHTTP()

	go func() {
		if err := pc.Run(ctx, c.PodSyncWorkers); err != nil && errors.Cause(err) != context.Canceled {
			log.G(ctx).Fatal(err)
		}
	}()

	if c.StartupTimeout > 0 {
		// If there is a startup timeout, it does two things:
		// 1. It causes the VK to shutdown if we haven't gotten into an operational state in a time period
		// 2. It prevents node advertisement from happening until we're in an operational state
		err = waitFor(ctx, c.StartupTimeout, pc.Ready())
		if err != nil {
			return err
		}
	}

	go func() {
		if err := nodeRunner.Run(ctx); err != nil {
			log.G(ctx).Fatal(err)
		}
	}()

	log.G(ctx).Info("Initialized")

	<-ctx.Done()
	return nil
}

func waitFor(ctx context.Context, time time.Duration, ready <-chan struct{}) error {
	ctx, cancel := context.WithTimeout(ctx, time)
	defer cancel()

	// Wait for the VK / PC close the the ready channel, or time out and return
	log.G(ctx).Info("Waiting for pod controller / VK to be ready")

	select {
	case <-ready:
		return nil
	case <-ctx.Done():
		return errors.Wrap(ctx.Err(), "Error while starting up VK")
	}
}

func newClient(configPath string, qps, burst int32) (*kubernetes.Clientset, error) {
	var config *rest.Config

	// Check if the kubeConfig file exists.
	if _, err := os.Stat(configPath); !os.IsNotExist(err) {
		// Get the kubeconfig from the filepath.
		config, err = clientcmd.BuildConfigFromFlags("", configPath)
		if err != nil {
			return nil, errors.Wrap(err, "error building client config")
		}
	} else {
		// Set to in-cluster config.
		config, err = rest.InClusterConfig()
		if err != nil {
			return nil, errors.Wrap(err, "error building in cluster config")
		}
	}

	if qps != 0 {
		config.QPS = float32(qps)
	}

	if burst != 0 {
		config.Burst = int(burst)
	}

	if masterURI := os.Getenv("MASTER_URI"); masterURI != "" {
		config.Host = masterURI
	}

	return kubernetes.NewForConfig(config)
}
