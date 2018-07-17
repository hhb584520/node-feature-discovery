/*
Copyright 2018 The Kubernetes Authors.

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

package cpu

import (
	"io/ioutil"
	"path"
	"regexp"

	"github.com/golang/glog"
)

// Implement FeatureSource interface
type Source struct{}

func (s Source) Name() string { return "cpu" }

func (s Source) Discover() ([]string, error) {
	features := []string{}

	// Check if hyper-threading seems to be enabled
	found, err := haveThreadSiblings()
	if err != nil {
		glog.Errorf("Failed to determine thread siblings: %v", err)
	} else if found {
		features = append(features, "logical-cpus")
	}
	return features, nil
}

// Check if any (online) CPUs have thread siblings
func haveThreadSiblings() (bool, error) {
	files, err := ioutil.ReadDir("/sys/devices/system/cpu/")
	if err != nil {
		return false, err
	}

	re := regexp.MustCompile(`^cpu\d`)
	for _, file := range files {
		if m := re.MatchString(file.Name()); m == true && file.Mode().IsDir() {
			// Try to read siblings from topology
			siblings, err := ioutil.ReadFile(path.Join("/sys/devices/system/cpu", file.Name(), "topology/thread_siblings_list"))
			if err != nil {
				continue
			}
			for _, char := range siblings {
				// If list separator found, we determine that there are multiple siblings
				if char == ',' || char == '-' {
					return true, nil
				}
			}
		}
	}
	// No siblings were found
	return false, nil
}
