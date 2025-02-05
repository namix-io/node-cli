module github.com/virtual-kubelet/node-cli

go 1.12

require (
	github.com/mitchellh/go-homedir v1.1.0
	github.com/pkg/errors v0.9.1
	github.com/sirupsen/logrus v1.8.1
	github.com/spf13/cobra v1.1.3
	github.com/spf13/pflag v1.0.5
	github.com/virtual-kubelet/virtual-kubelet v1.5.0
	go.opencensus.io v0.22.3
	gotest.tools v2.2.0+incompatible
	k8s.io/api v0.22.2
	k8s.io/apimachinery v0.22.2
	k8s.io/apiserver v0.22.2
	k8s.io/client-go v0.22.2
	k8s.io/klog v1.0.0
)

replace github.com/virtual-kubelet/virtual-kubelet => github.com/namix-io/virtual-kubelet v1.6.1

replace k8s.io/legacy-cloud-providers => k8s.io/legacy-cloud-providers v0.22.2

replace k8s.io/cloud-provider => k8s.io/cloud-provider v0.22.2

replace k8s.io/cli-runtime => k8s.io/cli-runtime v0.22.2

replace k8s.io/apiserver => k8s.io/apiserver v0.22.2

replace k8s.io/csi-translation-lib => k8s.io/csi-translation-lib v0.22.2

replace k8s.io/cri-api => k8s.io/cri-api v0.22.2

replace k8s.io/kube-aggregator => k8s.io/kube-aggregator v0.22.2

replace k8s.io/kubelet => k8s.io/kubelet v0.22.2

replace k8s.io/kube-controller-manager => k8s.io/kube-controller-manager v0.22.2

replace k8s.io/apimachinery => k8s.io/apimachinery v0.22.2

replace k8s.io/api => k8s.io/api v0.22.2

replace k8s.io/cluster-bootstrap => k8s.io/cluster-bootstrap v0.22.2

replace k8s.io/kube-proxy => k8s.io/kube-proxy v0.22.2

replace k8s.io/component-base => k8s.io/component-base v0.22.2

replace k8s.io/kube-scheduler => k8s.io/kube-scheduler v0.22.2

replace k8s.io/apiextensions-apiserver => k8s.io/apiextensions-apiserver v0.22.2

replace k8s.io/metrics => k8s.io/metrics v0.22.2

replace k8s.io/sample-apiserver => k8s.io/sample-apiserver v0.22.2

replace k8s.io/code-generator => k8s.io/code-generator v0.22.2

replace k8s.io/client-go => k8s.io/client-go v0.22.2

replace k8s.io/kubectl => k8s.io/kubectl v0.22.2
