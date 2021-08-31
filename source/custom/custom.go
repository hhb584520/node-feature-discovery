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

package custom

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"text/template"

	"k8s.io/klog/v2"
	"sigs.k8s.io/yaml"

	"sigs.k8s.io/node-feature-discovery/pkg/api/feature"
	nfdv1alpha1 "sigs.k8s.io/node-feature-discovery/pkg/apis/nfd/v1alpha1"
	"sigs.k8s.io/node-feature-discovery/pkg/utils"
	"sigs.k8s.io/node-feature-discovery/source"
	"sigs.k8s.io/node-feature-discovery/source/custom/rules"
)

const Name = "custom"

// LegacyMatcher contains the legacy custom rules.
type LegacyMatcher struct {
	PciID      *rules.PciIDRule      `json:"pciId,omitempty"`
	UsbID      *rules.UsbIDRule      `json:"usbId,omitempty"`
	LoadedKMod *rules.LoadedKModRule `json:"loadedKMod,omitempty"`
	CpuID      *rules.CpuIDRule      `json:"cpuId,omitempty"`
	Kconfig    *rules.KconfigRule    `json:"kConfig,omitempty"`
	Nodename   *rules.NodenameRule   `json:"nodename,omitempty"`
}

type LegacyRule struct {
	Name    string          `json:"name"`
	Value   *string         `json:"value,omitempty"`
	MatchOn []LegacyMatcher `json:"matchOn"`
}

type Rule struct {
	Name           string            `json:"name"`
	Labels         map[string]string `json:"labels"`
	LabelsTemplate string            `json:"labelsTemplate"`
	MatchFeatures  FeatureMatcher    `json:"matchFeatures"`
	MatchAny       []MatchAnyElem    `json:"matchAny"`

	labelsTemplate *template.Template
}

type MatchAnyElem struct {
	MatchFeatures FeatureMatcher
}

type FeatureMatcher []FeatureMatcherTerm

type FeatureMatcherTerm struct {
	Feature          string
	MatchExpressions nfdv1alpha1.MatchExpressionSet
}

type config []CustomRule

type CustomRule struct {
	*LegacyRule
	*Rule
}

// newDefaultConfig returns a new config with pre-populated defaults
func newDefaultConfig() *config {
	return &config{}
}

// customSource implements the LabelSource and ConfigurableSource interfaces.
type customSource struct {
	config *config
}

type legacyRule interface {
	Match() (bool, error)
}

// Singleton source instance
var (
	src                           = customSource{config: newDefaultConfig()}
	_   source.LabelSource        = &src
	_   source.ConfigurableSource = &src
)

// Name returns the name of the feature source
func (s *customSource) Name() string { return Name }

// NewConfig method of the LabelSource interface
func (s *customSource) NewConfig() source.Config { return newDefaultConfig() }

// GetConfig method of the LabelSource interface
func (s *customSource) GetConfig() source.Config { return s.config }

// SetConfig method of the LabelSource interface
func (s *customSource) SetConfig(c source.Config) {
	switch c.(type) {
	case *config:
	default:
		klog.Fatalf("invalid config type: %T", c)
	}

	// Parse template rules
	conf := c.(*config)
	for i, spec := range *conf {
		if spec.Rule != nil && spec.Rule.LabelsTemplate != "" {
			tmpl := template.Must(template.New("").Option("missingkey=error").Parse(spec.Rule.LabelsTemplate))
			(*conf)[i].Rule.labelsTemplate = tmpl
		}
	}

	s.config = conf
}

// Priority method of the LabelSource interface
func (s *customSource) Priority() int { return 10 }

// GetLabels method of the LabelSource interface
func (s *customSource) GetLabels() (source.FeatureLabels, error) {
	// Get raw features from all sources
	domainFeatures := make(map[string]*feature.DomainFeatures)
	for n, s := range source.GetAllFeatureSources() {
		domainFeatures[n] = s.GetFeatures()
	}

	labels := source.FeatureLabels{}
	allFeatureConfig := append(getStaticFeatureConfig(), *s.config...)
	allFeatureConfig = append(allFeatureConfig, getDirectoryFeatureConfig()...)
	utils.KlogDump(2, "custom features configuration:", "  ", allFeatureConfig)
	// Iterate over features
	for _, rule := range allFeatureConfig {
		ruleOut, err := rule.execute(domainFeatures)
		if err != nil {
			klog.Error(err)
			continue
		}

		for n, v := range ruleOut {
			labels[n] = v
		}
	}
	return labels, nil
}

// Process a single feature by Matching on the defined rules.
func (r *CustomRule) execute(features map[string]*feature.DomainFeatures) (map[string]string, error) {
	if r.LegacyRule != nil {
		ruleOut, err := r.LegacyRule.execute(features)
		if err != nil {
			return nil, fmt.Errorf("failed to execute legacy rule %s: %w", r.LegacyRule.Name, err)
		}
		return ruleOut, err
	}

	if r.Rule != nil {
		ruleOut, err := r.Rule.execute(features)
		if err != nil {
			return nil, fmt.Errorf("failed to execute rule %s: %w", r.Rule.Name, err)
		}
		return ruleOut, err
	}

	return nil, fmt.Errorf("BUG: an empty rule, this really should not happen")
}

// Process a single feature by Matching on the defined rules.
func (r *LegacyRule) execute(features map[string]*feature.DomainFeatures) (map[string]string, error) {
	if len(r.MatchOn) > 0 {
		// Logical OR over the legacy rules
		matched := false
		for _, matcher := range r.MatchOn {
			if m, err := matcher.match(); err != nil {
				return nil, err
			} else if m {
				matched = true
				break
			}
		}
		if !matched {
			return nil, nil
		}
	}

	value := "true"
	if r.Value != nil {
		value = *r.Value
	}
	return map[string]string{r.Name: value}, nil
}

func (r *Rule) execute(features map[string]*feature.DomainFeatures) (map[string]string, error) {
	ret := make(map[string]string)

	if len(r.MatchAny) > 0 {
		// Logical OR over the matchAny matchers
		matched := false
		for _, matcher := range r.MatchAny {
			if m, err := matcher.match(features); err != nil {
				return nil, err
			} else if m != nil {
				matched = true
				utils.KlogDump(4, "matches for matchAny "+r.Name, "  ", m)

				if r.labelsTemplate == nil {
					// No templating so we stop here (further matches would just
					// produce the same labels)
					break
				}
				if err := r.executeLabelsTemplate(m, ret); err != nil {
					return nil, err
				}

			}
		}
		if !matched {
			return nil, nil
		}
	}

	if len(r.MatchFeatures) > 0 {
		if m, err := r.MatchFeatures.match(features); err != nil {
			return nil, err
		} else if m == nil {
			return nil, nil
		} else {
			utils.KlogDump(4, "matches for matchFeatures "+r.Name, "  ", m)
			if err := r.executeLabelsTemplate(m, ret); err != nil {
				return nil, err
			}
		}
	}

	for k, v := range r.Labels {
		ret[k] = v
	}

	return ret, nil
}

func (r *Rule) executeLabelsTemplate(in matchedFeatures, out map[string]string) error {
	if r.labelsTemplate == nil {
		return nil
	}

	// Execute template to produce an array of labels
	var tmp bytes.Buffer
	if err := r.labelsTemplate.Execute(&tmp, in); err != nil {
		return err
	}
	expanded := tmp.String()

	// Split out individual labels
	for _, item := range strings.Split(expanded, "\n") {
		// Remove leading/trailing whitespace and skip empty lines
		if trimmed := strings.TrimSpace(item); trimmed != "" {
			split := strings.SplitN(trimmed, "=", 2)
			if len(split) == 1 {
				out[split[0]] = "true"
			} else {
				out[split[0]] = split[1]
			}
		}
	}
	return nil
}

type matchedFeatures map[string]domainMatchedFeatures

type domainMatchedFeatures map[string]interface{}

func (e *MatchAnyElem) match(features map[string]*feature.DomainFeatures) (matchedFeatures, error) {
	return e.MatchFeatures.match(features)
}

func (m *FeatureMatcher) match(features map[string]*feature.DomainFeatures) (matchedFeatures, error) {
	ret := make(matchedFeatures, len(*m))

	// Logical AND over the terms
	for _, term := range *m {
		split := strings.SplitN(term.Feature, ".", 2)
		if len(split) != 2 {
			return nil, fmt.Errorf("invalid feature %q: must be <domain>.<feature>", term.Feature)
		}
		domain := split[0]
		// Ignore case
		featureName := strings.ToLower(split[1])

		domainFeatures, ok := features[domain]
		if !ok {
			return nil, fmt.Errorf("unknown feature source/domain %q", domain)
		}

		if _, ok := ret[domain]; !ok {
			ret[domain] = make(domainMatchedFeatures)
		}

		var m bool
		var e error
		if f, ok := domainFeatures.Keys[featureName]; ok {
			v, err := term.MatchExpressions.MatchGetKeys(f.Elements)
			m = len(v) > 0
			e = err
			ret[domain][featureName] = v
		} else if f, ok := domainFeatures.Values[featureName]; ok {
			v, err := term.MatchExpressions.MatchGetValues(f.Elements)
			m = len(v) > 0
			e = err
			ret[domain][featureName] = v
		} else if f, ok := domainFeatures.Instances[featureName]; ok {
			v, err := term.MatchExpressions.MatchGetInstances(f.Elements)
			m = len(v) > 0
			e = err
			ret[domain][featureName] = v
		} else {
			return nil, fmt.Errorf("%q feature of source/domain %q not available", featureName, domain)
		}

		if e != nil {
			return nil, e
		} else if !m {
			return nil, nil
		}
	}
	return ret, nil
}

func (m *LegacyMatcher) match() (bool, error) {
	allRules := []legacyRule{
		m.PciID,
		m.UsbID,
		m.LoadedKMod,
		m.CpuID,
		m.Kconfig,
		m.Nodename,
	}

	// return true, nil if all rules match
	matchRules := func(rules []legacyRule) (bool, error) {
		for _, rule := range rules {
			if reflect.ValueOf(rule).IsNil() {
				continue
			}
			if match, err := rule.Match(); err != nil {
				return false, err
			} else if !match {
				return false, nil
			}
		}
		return true, nil
	}

	return matchRules(allRules)
}

// UnmarshalJSON implements the Unmarshaler interface from "encoding/json"
func (c *CustomRule) UnmarshalJSON(data []byte) error {
	// Do a raw parse to determine if this is a legacy rule
	raw := map[string]json.RawMessage{}
	err := yaml.Unmarshal(data, &raw)
	if err != nil {
		return err
	}

	for k := range raw {
		if strings.ToLower(k) == "matchon" {
			return yaml.Unmarshal(data, &c.LegacyRule)
		}
	}

	return yaml.Unmarshal(data, &c.Rule)
}

// MarshalJSON implements the Marshaler interface from "encoding/json"
func (c *CustomRule) MarshalJSON() ([]byte, error) {
	if c.LegacyRule != nil {
		return json.Marshal(c.LegacyRule)
	}
	return json.Marshal(c.Rule)
}

func init() {
	source.Register(&src)
}
