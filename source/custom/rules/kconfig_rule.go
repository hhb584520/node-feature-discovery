/*
Copyright 2020-2021 The Kubernetes Authors.

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

	nfdv1alpha1 "sigs.k8s.io/node-feature-discovery/pkg/apis/nfd/v1alpha1"
	"sigs.k8s.io/node-feature-discovery/source/kernel"
)

// KconfigRule implements Rule for the custom source
type KconfigRule struct {
	nfdv1alpha1.MatchExpressionSet
}

func (r *KconfigRule) Match() (bool, error) {
	options := kernel.GetLegacyKconfig()
	if options == nil {
		return false, fmt.Errorf("kernel config options not available")
	}
	return r.MatchValues(options)
}
