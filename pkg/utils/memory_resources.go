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

package utils

import (
	"fmt"
	"io/ioutil"
	"path/filepath"
	"strconv"
	"strings"

	v1 "k8s.io/api/core/v1"
	"k8s.io/klog/v2"
)

var (
	sysBusNodeBasepath = "/sys/bus/node/devices"
)

// NumaMemoryResources contains information of the memory resources per NUMA
// nodes of the system.
type NumaMemoryResources map[int]MemoryResourceInfo

// MemoryResourceInfo holds information of memory resources per resource type.
type MemoryResourceInfo map[v1.ResourceName]int64

// GetNumaMemoryResources returns total amount of memory and hugepages under NUMA nodes
func GetNumaMemoryResources() (NumaMemoryResources, error) {
	nodes, err := ioutil.ReadDir(sysBusNodeBasepath)
	if err != nil {
		return nil, err
	}

	memoryResources := make(NumaMemoryResources, len(nodes))
	for _, n := range nodes {
		numaNode := n.Name()
		nodeID, err := strconv.Atoi(numaNode[4:])
		if err != nil {
			return nil, fmt.Errorf("failed to parse NUMA node ID of %w", numaNode)
		}

		info := make(MemoryResourceInfo)

		// Get total memory
		nodeTotalMemory, err := readTotalMemoryFromMeminfo(filepath.Join(sysBusNodeBasepath, numaNode, "meminfo"))
		if err != nil {
			return nil, err
		}
		info[v1.ResourceMemory] = nodeTotalMemory

		// Get hugepages
		hugepageCounts, err := getHugepagesCounts(filepath.Join(sysBusNodeBasepath, numaNode, "hugepages"))
		if err != nil {
			return nil, err
		}
		if nr, ok := hugepageCounts["2048kB"]; ok {
			info[v1.ResourceHugePagesPrefix+"hugepages-2Mi"] = nr * 2048 * 1024
		}
		if nr, ok := hugepageCounts["1048576kB"]; ok {
			info[v1.ResourceHugePagesPrefix+"hugepages-1Gi"] = nr * 1048576 * 1024
		}

		memoryResources[nodeID] = info
	}

	return memoryResources, nil
}

func getHugepagesCounts(path string) (map[string]int64, error) {
	entries, err := ioutil.ReadDir(path)
	if err != nil {
		return nil, err
	}

	var hugepagesCounts map[string]int64
	for _, entry := range entries {
		split := strings.SplitN(entry.Name(), "-", 2)
		if len(split) != 2 || split[0] != "hugepages" {
			klog.Warningf("malformed hugepages entry %q", entry.Name())
			continue
		}

		data, err := ioutil.ReadFile(filepath.Join(path, entry.Name(), "nr_hugepages"))
		if err != nil {
			return nil, err
		}

		nr, err := strconv.ParseInt(strings.TrimSpace(string(data)), 10, 64)
		if err != nil {
			return nil, err
		}

		hugepagesCounts[split[0]] = nr
	}

	return hugepagesCounts, nil
}

func readTotalMemoryFromMeminfo(path string) (int64, error) {
	data, err := ioutil.ReadFile(path)
	if err != nil {
		return -1, err
	}

	for _, line := range strings.Split(string(data), "\n") {
		split := strings.SplitN(line, ":", 2)
		if len(split) != 2 {
			continue
		}

		if split[0] == "MemTotal" {
			memValue := strings.Trim(split[1], "\t\n kB")
			convertedValue, err := strconv.ParseInt(memValue, 10, 64)
			if err != nil {
				return -1, fmt.Errorf("failed to convert value: %v", memValue)
			}

			// return information in bytes
			return 1024 * convertedValue, nil
		}
	}

	return -1, fmt.Errorf("failed to find MemTotal field under the file %q", path)
}
