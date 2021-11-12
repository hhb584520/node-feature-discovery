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

package v1alpha1

import (
	"strings"
	"text/template"

	"bytes"
	"fmt"

	"sigs.k8s.io/node-feature-discovery/pkg/api/feature"
	"sigs.k8s.io/node-feature-discovery/pkg/utils"
)

// Execute the rule against a set of input features.
func (r *Rule) Execute(features map[string]*feature.DomainFeatures) (map[string]string, error) {
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
	if r.LabelsTemplate == "" {
		return nil
	}

	if r.labelsTemplate == nil {
		t, err := newTemplateHelper(r.LabelsTemplate)
		if err != nil {
			return err
		}
		r.labelsTemplate = t
	}

	labels, err := r.labelsTemplate.expandMap(in)
	if err != nil {
		return err
	}
	for k, v := range labels {
		out[k] = v
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

type templateHelper struct {
	template *template.Template
}

func newTemplateHelper(name string) (*templateHelper, error) {
	tmpl, err := template.New("").Option("missingkey=error").Parse(name)
	if err != nil {
		return nil, fmt.Errorf("invalid template: %w", err)
	}
	return &templateHelper{template: tmpl}, nil
}

// DeepCopy is a stub to augment the auto-generated code
func (in *templateHelper) DeepCopy() *templateHelper {
	if in == nil {
		return nil
	}
	out := new(templateHelper)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is a stub to augment the auto-generated code
func (in *templateHelper) DeepCopyInto(out *templateHelper) {
	// HACK: just re-use the template
	out.template = in.template
}

func (h *templateHelper) execute(data interface{}) (string, error) {
	var tmp bytes.Buffer
	if err := h.template.Execute(&tmp, data); err != nil {
		return "", err
	}
	return tmp.String(), nil
}

// expandMap is a helper for expanding a template in to a map of strings. Data
// after executing the template is expexted to be key=value pairs separated by
// newlines.
func (h *templateHelper) expandMap(data interface{}) (map[string]string, error) {
	expanded, err := h.execute(data)
	if err != nil {
		return nil, err
	}

	// Split out individual key-value pairs
	out := make(map[string]string)
	for _, item := range strings.Split(expanded, "\n") {
		// Remove leading/trailing whitespace and skip empty lines
		if trimmed := strings.TrimSpace(item); trimmed != "" {
			split := strings.SplitN(trimmed, "=", 2)
			if len(split) == 1 {
				out[split[0]] = ""
			} else {
				out[split[0]] = split[1]
			}
		}
	}
	return out, nil
}
