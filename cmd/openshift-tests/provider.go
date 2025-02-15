package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/onsi/ginkgo"
	"github.com/onsi/gomega"

	reale2e "k8s.io/kubernetes/test/e2e"
	e2e "k8s.io/kubernetes/test/e2e/framework"

	exutil "github.com/openshift/origin/test/extended/util"
	exutilcloud "github.com/openshift/origin/test/extended/util/cloud"

	// Initialize baremetal as a provider
	_ "github.com/openshift/origin/test/extended/util/baremetal"

	// Initialize ovirt as a provider
	_ "github.com/openshift/origin/test/extended/util/ovirt"

	// Initialize kubevirt as a provider
	_ "github.com/openshift/origin/test/extended/util/kubevirt"

	// these are loading important global flags that we need to get and set
	_ "k8s.io/kubernetes/test/e2e"
	_ "k8s.io/kubernetes/test/e2e/lifecycle"
)

type TestNameMatchesFunc func(name string) bool

func initializeTestFramework(context *e2e.TestContextType, config *exutilcloud.ClusterConfiguration, dryRun bool) error {
	// update context with loaded config
	context.Provider = config.ProviderName
	context.CloudConfig = e2e.CloudConfig{
		ProjectID:   config.ProjectID,
		Region:      config.Region,
		Zone:        config.Zone,
		Zones:       config.Zones,
		NumNodes:    config.NumNodes,
		MultiMaster: config.MultiMaster,
		MultiZone:   config.MultiZone,
		ConfigFile:  config.ConfigFile,
	}
	context.AllowedNotReadyNodes = -1
	context.MinStartupPods = -1
	context.MaxNodesToGather = 0
	reale2e.SetViperConfig(os.Getenv("VIPERCONFIG"))

	// allow the CSI tests to access test data, but only briefly
	// TODO: ideally CSI would not use any of these test methods
	var err error
	exutil.WithCleanup(func() { err = initCSITests(dryRun) })
	if err != nil {
		return err
	}

	if err := exutil.InitTest(dryRun); err != nil {
		return err
	}
	gomega.RegisterFailHandler(ginkgo.Fail)

	e2e.AfterReadingAllFlags(context)
	context.DumpLogsOnFailure = true
	return nil
}

func getProviderMatchFn(config *exutilcloud.ClusterConfiguration) TestNameMatchesFunc {
	// given the configuration we have loaded, skip tests that our provider should exclude
	// or our network plugin should exclude
	var skips []string
	skips = append(skips, fmt.Sprintf("[Skipped:%s]", config.ProviderName))
	for _, id := range config.NetworkPluginIDs {
		skips = append(skips, fmt.Sprintf("[Skipped:Network/%s]", id))
	}
	matchFn := func(name string) bool {
		for _, skip := range skips {
			if strings.Contains(name, skip) {
				return false
			}
		}
		return true
	}
	return matchFn
}

func decodeProvider(provider string, dryRun, discover bool) (*exutilcloud.ClusterConfiguration, error) {
	switch provider {
	case "none":
		return &exutilcloud.ClusterConfiguration{ProviderName: "skeleton"}, nil

	case "":
		if _, ok := os.LookupEnv("KUBE_SSH_USER"); ok {
			if _, ok := os.LookupEnv("LOCAL_SSH_KEY"); ok {
				return &exutilcloud.ClusterConfiguration{ProviderName: "local"}, nil
			}
		}
		if dryRun {
			return &exutilcloud.ClusterConfiguration{ProviderName: "skeleton"}, nil
		}
		fallthrough

	case "azure", "aws", "baremetal", "gce", "vsphere":
		clientConfig, err := e2e.LoadConfig(true)
		if err != nil {
			return nil, err
		}
		config, err := exutilcloud.LoadConfig(clientConfig)
		if err != nil {
			return nil, err
		}
		if len(config.ProviderName) == 0 {
			config.ProviderName = "skeleton"
		}
		return config, nil

	default:
		var providerInfo struct{ Type string }
		if err := json.Unmarshal([]byte(provider), &providerInfo); err != nil {
			return nil, fmt.Errorf("provider must be a JSON object with the 'type' key at a minimum: %v", err)
		}
		if len(providerInfo.Type) == 0 {
			return nil, fmt.Errorf("provider must be a JSON object with the 'type' key")
		}
		var cloudConfig e2e.CloudConfig
		if err := json.Unmarshal([]byte(provider), &cloudConfig); err != nil {
			return nil, fmt.Errorf("provider must decode into the cloud config object: %v", err)
		}

		// attempt to load the default config, then overwrite with any values from the passed
		// object that can be overriden
		var config *exutilcloud.ClusterConfiguration
		if discover {
			if clientConfig, err := e2e.LoadConfig(true); err == nil {
				config, _ = exutilcloud.LoadConfig(clientConfig)
			}
		}
		if config == nil {
			config = &exutilcloud.ClusterConfiguration{
				ProviderName: providerInfo.Type,
				ProjectID:    cloudConfig.ProjectID,
				Region:       cloudConfig.Region,
				Zone:         cloudConfig.Zone,
				Zones:        cloudConfig.Zones,
				NumNodes:     cloudConfig.NumNodes,
				MultiMaster:  cloudConfig.MultiMaster,
				MultiZone:    cloudConfig.MultiZone,
				ConfigFile:   cloudConfig.ConfigFile,
			}
		} else {
			config.ProviderName = providerInfo.Type
			if len(cloudConfig.ProjectID) > 0 {
				config.ProjectID = cloudConfig.ProjectID
			}
			if len(cloudConfig.Region) > 0 {
				config.Region = cloudConfig.Region
			}
			if len(cloudConfig.Zone) > 0 {
				config.Zone = cloudConfig.Zone
			}
			if len(cloudConfig.Zones) > 0 {
				config.Zones = cloudConfig.Zones
			}
			if len(cloudConfig.ConfigFile) > 0 {
				config.ConfigFile = cloudConfig.ConfigFile
			}
			if cloudConfig.NumNodes > 0 {
				config.NumNodes = cloudConfig.NumNodes
			}
		}
		return config, nil
	}
}
