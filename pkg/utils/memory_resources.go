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
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/klog/v2"
)

var (
	sysBusNodeDevices = "/sys/bus/node/devices"
)

const (
	HugepageSize2Mi = 2048
	HugepageSize1Gi = 1048576
)

type MemoryResourcePerNUMA map[int]map[v1.ResourceName]int64

// GetMemoryResourceCounters returns total amount of memory and hugepages under NUMA nodes
func GetMemoryResourceCounters() (MemoryResourcePerNUMA, error) {
	numaNodeIDs, err := getNUMANodesIDs()
	if err != nil {
		return nil, err
	}

	memoryResources := make(MemoryResourcePerNUMA)
	for _, numaNodeID := range numaNodeIDs {
		memoryResources[numaNodeID] = map[v1.ResourceName]int64{}
		numaNodeFolderName := fmt.Sprintf("node%d", numaNodeID)

		// get the NUMA node total memory
		memInfoFilePath := filepath.Join(sysBusNodeDevices, numaNodeFolderName, "meminfo")
		nodeTotalMemory, err := readTotalMemoryFromMeminfo(memInfoFilePath)
		if err != nil {
			return nil, err
		}
		memoryResources[numaNodeID][v1.ResourceMemory] = nodeTotalMemory

		// get the NUMA node hugepages
		hugepagesFolderPath := filepath.Join(sysBusNodeDevices, numaNodeFolderName, "hugepages")
		hugepageSizes, err := getHugepagesSizes(hugepagesFolderPath)
		if err != nil {
			return nil, err
		}

		for _, hugepageSize := range hugepageSizes {
			hugepagesSizeFolderName := fmt.Sprintf("hugepages-%dkB", hugepageSize)
			hugepagesTotalFile := filepath.Join(
				hugepagesFolderPath,
				hugepagesSizeFolderName,
				"nr_hugepages",
			)
			nodeHugepagesTotal, err := readIntFromFile(hugepagesTotalFile)
			if err != nil {
				return nil, err
			}

			hugepagesResource := hugepageResourceNameFromSize(hugepageSize)
			// save the total amount of memory allocated for hugepages in bytes
			memoryResources[numaNodeID][hugepagesResource] = int64(nodeHugepagesTotal * hugepageSize * 1024)
		}
	}

	return memoryResources, nil
}

func getNUMANodesIDs() ([]int, error) {
	entries, err := ioutil.ReadDir(sysBusNodeDevices)
	if err != nil {
		return nil, err
	}

	var numaNodesIDs []int
	for _, entry := range entries {
		entryName := entry.Name()
		if entry.IsDir() && strings.HasPrefix(entryName, "node") {
			nodeID, err := strconv.Atoi(entryName[4:])
			if err != nil {
				klog.Warningf("cannot detect the node ID for %q", entryName)
				continue
			}

			numaNodesIDs = append(numaNodesIDs, nodeID)
		}
	}

	return numaNodesIDs, nil
}

func getHugepagesSizes(path string) ([]int, error) {
	entries, err := ioutil.ReadDir(path)
	if err != nil {
		return nil, err
	}

	var hugepageSizes []int
	for _, entry := range entries {
		entryName := entry.Name()
		var hugepageSizeKB int
		if n, err := fmt.Sscanf(entryName, "hugepages-%dkB", &hugepageSizeKB); n != 1 || err != nil {
			klog.Warningf("malformed hugepages entry %q", entryName)
			continue
		}

		hugepageSizes = append(hugepageSizes, hugepageSizeKB)
	}

	return hugepageSizes, nil
}

func hugepageResourceNameFromSize(sizeKB int) v1.ResourceName {
	qty := resource.NewQuantity(int64(sizeKB*1024), resource.BinarySI)
	return v1.ResourceName("hugepages-" + qty.String())
}

func readIntFromFile(path string) (int, error) {
	data, err := ioutil.ReadFile(path)
	if err != nil {
		return -1, err
	}
	return strconv.Atoi(strings.TrimSpace(string(data)))
}

func readTotalMemoryFromMeminfo(path string) (int64, error) {
	data, err := ioutil.ReadFile(path)
	if err != nil {
		return -1, err
	}

	for _, line := range strings.Split(string(data), "\n") {
		if !strings.Contains(line, "MemTotal") {
			continue
		}

		memTotal := strings.Split(line, ":")
		if len(memTotal) != 2 {
			return -1, fmt.Errorf("MemTotal has unexpected format: %s", line)
		}

		memValue := strings.Trim(memTotal[1], "\t\n kB")
		convertedValue, err := strconv.ParseInt(memValue, 10, 64)
		if err != nil {
			return -1, fmt.Errorf("failed to convert value: %v", memValue)
		}

		// return information in bytes
		return 1024 * convertedValue, nil
	}

	return -1, fmt.Errorf("failed to find MemTotal field under the file %q", path)
}
