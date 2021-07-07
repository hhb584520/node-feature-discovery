/*
Copyright 2021 The Kubernetes Authors.

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

package rules

import (
	"fmt"
	"regexp"

	"k8s.io/klog/v2"

	"sigs.k8s.io/node-feature-discovery/source"
	"sigs.k8s.io/node-feature-discovery/source/system"
)

// NodenameRule matches on nodenames configured in a ConfigMap
type NodenameRule []string

// Force implementation of Rule
var _ Rule = NodenameRule{}

func (n NodenameRule) Match() (bool, error) {
	nodeName, ok := source.GetFeatureSource("system").GetFeatures().Values[system.NameFeature].Features["nodename"]
	if !ok {
		return false, fmt.Errorf("node name not available")
	}

	for _, nodenamePattern := range n {
		klog.V(1).Infof("matchNodename %s", nodenamePattern)
		match, err := regexp.MatchString(nodenamePattern, nodeName)
		if err != nil {
			klog.Errorf("nodename rule: invalid nodename regexp %q: %v", nodenamePattern, err)
			continue
		}
		if !match {
			klog.V(2).Infof("nodename rule: No match for pattern %q with node %q", nodenamePattern, nodeName)
			continue
		}
		klog.V(2).Infof("nodename rule: Match for pattern %q with node %q", nodenamePattern, nodeName)
		return true, nil
	}
	return false, nil
}
