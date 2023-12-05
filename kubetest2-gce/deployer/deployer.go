/*
Copyright 2020 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// Package deployer implements the kubetest2 GKE deployer
package deployer

import (
	goflag "flag"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/octago/sflags/gen/gpflag"
	"github.com/spf13/pflag"

	"k8s.io/klog/v2"

	"sigs.k8s.io/boskos/client"

	"sigs.k8s.io/kubetest2/kubetest2-gce/deployer/options"
	"sigs.k8s.io/kubetest2/pkg/artifacts"
	"sigs.k8s.io/kubetest2/pkg/build"
	"sigs.k8s.io/kubetest2/pkg/types"
)

// Name is the name of the deployer
const Name = "gce"

var GitTag string

type Deployer struct {
	// generic parts
	commonOptions types.Options

	BuildOptions *options.BuildOptions

	doInit sync.Once

	kubeconfigPath string
	kubectlPath    string
	logsDir        string

	// boskos struct field will be non-nil when the deployer is
	// using boskos to acquire a GCP project
	boskos *client.Client

	// this channel serves as a signal channel for the hearbeat goroutine
	// so that it can be explicitly closed
	boskosHeartbeatClose chan struct{}

	// instancePrefix is set for a mandatory env and for firewall rule creation
	// see buildEnv() and nodeTag()
	instancePrefix string
	// network is set for firewall rule creation, see buildEnv() and firewall.go
	network string

	// env is passed to buildEnv() function, many env variables are set by other flags
	Env []string `desc:"A list on env variables to pass to the kube-*.sh scripts"`

	BoskosAcquireTimeoutSeconds    int    `desc:"How long (in seconds) to hang on a request to Boskos to acquire a resource before erroring."`
	BoskosHeartbeatIntervalSeconds int    `desc:"How often (in seconds) to send a heartbeat to Boskos to hold the acquired resource. 0 means no heartbeat."`
	RepoRoot                       string `desc:"The path to the root of the local kubernetes/cloud-provider-gcp repo. Necessary to call certain scripts. Defaults to the current directory. If operating in legacy mode, this should be set to the local kubernetes/kubernetes repo."`
	GCPProject                     string `desc:"GCP Project to create VMs in. If unset, the deployer will attempt to get a project from boskos."`
	GCPZone                        string `desc:"GCP Zone to create VMs in. If unset, kube-up.sh and kube-down.sh defaults apply."`
	EnableComputeAPI               bool   `desc:"If set, the deployer will enable the compute API for the project during the Up phase. This is necessary if the project has not been used before. WARNING: The currently configured GCP account must have permission to enable this API on the configured project."`
	OverwriteLogsDir               bool   `desc:"If set, will overwrite an existing logs directory if one is encountered during dumping of logs. Useful when runnning tests locally."`
	BoskosLocation                 string `desc:"If set, manually specifies the location of the boskos server. If unset and boskos is needed, defaults to http://boskos.test-pods.svc.cluster.local."`
	LegacyMode                     bool   `desc:"Set if the provided repo root is the kubernetes/kubernetes repo and not kubernetes/cloud-provider-gcp."`
	NumNodes                       int    `desc:"The number of nodes in the cluster."`

	EnableCacheMutationDetector bool   `desc:"Sets the environment variable ENABLE_CACHE_MUTATION_DETECTOR=true during deployment. This should cause a panic if anything mutates a shared informer cache."`
	RuntimeConfig               string `desc:"Sets the KUBE_RUNTIME_CONFIG environment variable during deployment."`
	EnablePodSecurityPolicy     bool   `desc:"Sets the environment variable ENABLE_POD_SECURITY_POLICY=true during deployment."`
	CreateCustomNetwork         bool   `desc:"Sets the environment variable CREATE_CUSTOM_NETWORK=true during deployment."`
	NodeScopes                  string `desc:"Sets the NODE_SCOPES environment variable during deployment."`
	NodeServiceAccount          string `desc:"Sets the KUBE_GCE_NODE_SERVICE_ACCOUNT environment variable during deployment."`
	CloudProvider               string `desc:"Sets the CLOUD_PROVIDER environment variable during deployment."`
	FeatureGates                string `desc:"Sets the KUBE_FEATURE_GATES environment variable during deployment."`

	MasterSize string `desc:"Sets the MASTER_SIZE environment variable during deployment."`
	NodeSize   string `desc:"Sets the NODE_SIZE environment variable during deployment."`

	IngressGCEImage string `desc:"Sets the ingress-gce image used for the Ingress and Loadbalancer controller."`
}

// pseudoUniqueSubstring returns a substring of a UUID
// that can be reasonably used in resource names
// where length is constrained
// e.g https://cloud.google.com/compute/docs/naming-resources
// but still retain as much uniqueness as possible
// also easily lets us tie it back to a run
func pseudoUniqueSubstring(uuid string) string {
	// both KUBETEST2_RUN_ID and PROW_JOB_ID uuids are generated
	// following RFC 4122 https://tools.ietf.org/html/rfc4122
	// e.g. 09a2565a-7ac6-11eb-a603-2218f636630c
	// extract the first 13 characters (09a2565a-7ac6) as they are the ones that depend on
	// timestamp and has the best avalanche effect (https://en.wikipedia.org/wiki/Avalanche_effect)
	// as compared to the other bytes
	// 13 characters is also <= the no. of character being used previously
	const maxResourceNamePrefixLength = 13
	if len(uuid) <= maxResourceNamePrefixLength {
		return uuid
	}
	return uuid[:maxResourceNamePrefixLength]
}

// New implements deployer.New for gce
func New(opts types.Options) (types.Deployer, *pflag.FlagSet) {
	d := &Deployer{
		commonOptions: opts,
		BuildOptions: &options.BuildOptions{
			CommonBuildOptions: &build.Options{
				Builder:         &build.NoopBuilder{},
				Stager:          &build.NoopStager{},
				Strategy:        "make",
				TargetBuildArch: "linux/amd64",
			},
		},
		kubeconfigPath:       filepath.Join(opts.RunDir(), "kubetest2-kubeconfig"),
		logsDir:              filepath.Join(artifacts.BaseDir(), "cluster-logs"),
		boskosHeartbeatClose: make(chan struct{}),
		// names need to start with an alphabet
		instancePrefix:                 "kt2-" + pseudoUniqueSubstring(opts.RunID()),
		network:                        "kt2-" + pseudoUniqueSubstring(opts.RunID()),
		BoskosAcquireTimeoutSeconds:    5 * 60,
		BoskosHeartbeatIntervalSeconds: 5 * 60,
		BoskosLocation:                 "http://boskos.test-pods.svc.cluster.local.",
		NumNodes:                       3,
	}

	flagSet, err := gpflag.Parse(d)
	if err != nil {
		klog.Fatalf("couldn't parse flagset for Deployer struct: %s", err)
	}

	flagSet.AddGoFlagSet(goflag.CommandLine)

	// register flags and return
	return d, flagSet
}

// assert that New implements types.NewDeployer
var _ types.NewDeployer = New

// assert that deployer implements types.Deployer
var _ types.Deployer = &Deployer{}

func (d *Deployer) Provider() string {
	return Name
}

func (d *Deployer) Version() string {
	return GitTag
}

func (d *Deployer) Kubeconfig() (string, error) {
	_, err := os.Stat(d.kubeconfigPath)
	if os.IsNotExist(err) {
		return "", fmt.Errorf("kubeconfig does not exist at: %s", d.kubeconfigPath)
	}
	if err != nil {
		return "", fmt.Errorf("unknown error when checking for kubeconfig at %s: %s", d.kubeconfigPath, err)
	}

	return d.kubeconfigPath, nil
}
