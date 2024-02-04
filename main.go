/*
Copyright 2016 The Kubernetes Authors.
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

// The external controller manager is responsible for running controller loops that
// are cloud provider dependent. It uses the API to listen to new events on resources.

package main

import (
	"fmt"
	"os"

	"github.com/kube-vip/kube-vip-cloud-provider/pkg/provider"
	"github.com/spf13/pflag"
	"k8s.io/apimachinery/pkg/util/wait"
	cloudprovider "k8s.io/cloud-provider"
	"k8s.io/cloud-provider/app"
	"k8s.io/cloud-provider/app/config"
	"k8s.io/cloud-provider/names"
	"k8s.io/cloud-provider/options"
	cliflag "k8s.io/component-base/cli/flag"
	"k8s.io/component-base/logs"
	_ "k8s.io/component-base/metrics/prometheus/clientgo" // for client metric registration
	_ "k8s.io/component-base/metrics/prometheus/version"  // for version metric registration
	"k8s.io/klog"
)

func main() {
	// a random number generator will be seeded automatically

	logs.InitLogs()
	defer logs.FlushLogs()

	opts, err := options.NewCloudControllerManagerOptions()
	if err != nil {
		klog.Fatalf("unable to initialize command options: %v", err)
	}

	fss := cliflag.NamedFlagSets{}

	command := app.NewCloudControllerManagerCommand(opts, cloudInitializer, controllerInitializers(), names.CCMControllerAliases(), fss, wait.NeverStop)

	command.Flags().BoolVar(&provider.OutSideCluster, "OutSideCluster", false, "Start Controller outside of cluster")

	// Set static flags for which we know the values.
	command.Flags().VisitAll(func(fl *pflag.Flag) {
		var err error
		switch fl.Name {
		case "allow-untagged-cloud",
			// Untagged clouds must be enabled explicitly as they were once marked
			// deprecated. See
			// https://github.com/kubernetes/cloud-provider/issues/12 for an ongoing
			// discussion on whether that is to be changed or not.
			"authentication-skip-lookup":
			// Prevent reaching out to an authentication-related ConfigMap that
			// we do not need, and thus do not intend to create RBAC permissions
			// for. See also
			// https://github.com/digitalocean/digitalocean-cloud-controller-manager/issues/217
			// and https://github.com/kubernetes/cloud-provider/issues/29.
			err = fl.Value.Set("true")
		case "cloud-provider":
			// Specify the name we register our own cloud provider implementation
			// for.
			err = fl.Value.Set(provider.ProviderName)
		}
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to set flag %q: %s\n", fl.Name, err)
			os.Exit(1)
		}
	})

	if err := command.Execute(); err != nil {
		os.Exit(1)
	}
}

// only enable service controller
func controllerInitializers() map[string]app.ControllerInitFuncConstructor {
	return map[string]app.ControllerInitFuncConstructor{
		names.ServiceLBController: app.DefaultInitFuncConstructors[names.ServiceLBController],
	}
}

func cloudInitializer(_ *config.CompletedConfig) cloudprovider.Interface {
	// initialize cloud provider with the cloud provider name and config file provided
	cloud, err := cloudprovider.InitCloudProvider(provider.ProviderName, "")
	if err != nil {
		klog.Fatalf("Cloud provider could not be initialized: %v", err)
	}
	if cloud == nil {
		klog.Fatalf("Cloud provider is nil")
	}

	return cloud
}
