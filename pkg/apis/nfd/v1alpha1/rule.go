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
	"text/template"

	"bytes"
	"fmt"
)

func (r *Rule) executeLabelsTemplate(data interface{}) (string, error) {
	if r.LabelsTemplate == "" {
		return "", nil
	}

	if r.labelsTemplate == nil {
		t, err := newTemplateHelper(r.LabelsTemplate)
		if err != nil {
			return "", err
		}
		r.labelsTemplate = t
	}

	return r.labelsTemplate.execute(data)
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
