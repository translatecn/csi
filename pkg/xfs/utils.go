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

package xfs

import (
	"fmt"
	"io/ioutil"
	"math"
	"os"
	"strconv"
	"strings"
	"sync"
)

const (
	// ReservedExportID ExportID = 152 is being reserved
	// so that 152.152 is not used as filesystem_id in nfs-ganesha export configuration
	// 152.152 is the default pseudo root filesystem ID
	// Ref: https://github.com/kubernetes-sigs/nfs-ganesha-server-and-external-provisioner/issues/7
	ReservedExportID = 152
)

// generateID generates a unique exportID to assign an export
func generateID(mutex *sync.Mutex, ds map[string]Info) uint16 {
	mutex.Lock()
	ids := map[uint16]bool{}

	for _, d := range ds {
		ids[d.ID] = true
	}

	id := uint16(1)
	for ; id <= math.MaxUint16; id++ {
		if _, ok := ids[id]; !ok {
			if id == ReservedExportID {
				continue
			}
			break
		}
	}
	ids[id] = true
	mutex.Unlock()
	return id
}

func deleteDirectory(mutex *sync.Mutex, ds map[string]Info, d string) {
	mutex.Lock()
	delete(ds, d)
	mutex.Unlock()
}

// getExistingIDs populates a map with existing ids found in the given config
// file using the given regexp. Regexp must have a "digits" submatch.
func getExistingIDs(config string) (map[string]Info, error) {
	ids := map[string]Info{}

	read, err := ioutil.ReadFile(config)
	if err != nil {
		return ids, err
	}

	lines := strings.Split(string(read), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if len(line) == 0 {
			continue
		}
		match := strings.Split(line, ":")
		if len(match) != 3 {
			continue
		}
		projectID, _ := strconv.ParseUint(match[0], 10, 16)
		directory := match[1]
		bhard := match[2]

		// If directory referenced by projects file no longer exists, don't set a
		// quota for it: will fail
		if _, err := os.Stat(directory); os.IsNotExist(err) {
			continue
		}

		ids[directory] = Info{
			ID:  uint16(projectID),
			Cap: bhard,
		}

	}

	return ids, nil
}

func ConvertBytesToString(bytes float64) string {
	units := []string{"B", "K", "M", "G", "T", "P", "E"}

	unitIndex := 0
	for bytes >= 1024 && unitIndex < len(units)-1 {
		bytes /= 1024
		unitIndex++
	}
	return fmt.Sprintf("%d%s", int64(bytes), units[unitIndex])
}
