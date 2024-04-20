package xfs

import (
	"bytes"
	"fmt"
	"github.com/kubernetes-sigs/sig-storage-lib-external-provisioner/mount"
	"io/ioutil"
	klog "k8s.io/klog/v2"
	"os"
	"os/exec"
	"path"
	"strconv"
	"strings"
	"sync"
)

type xfsQuotaer struct {
	xfsPath string

	// The file where we store mappings between project ids and directories, and
	// each project's quota limit information, for backup.
	// Similar to http://man7.org/linux/man-pages/man5/projects.5.html
	projectsFile string
	directories  Directories
	mapMutex     *sync.Mutex
	fileMutex    *sync.Mutex
}
type Directories map[string]Info
type Info struct {
	ID  uint16
	Cap string
}

func (d Directories) String() string {
	buf := make([]byte, 1000)
	res := bytes.NewBuffer(buf)
	for directory, v := range d {
		res.WriteString(strconv.FormatUint(uint64(v.ID), 10))
		res.WriteString(":")
		res.WriteString(directory)
		res.WriteString(":")
		res.WriteString(v.Cap)
		res.WriteString("\n")
	}
	return res.String()
}

type IxfsQuotaer interface {
	AddProject(directory, bhard string) (string, uint16, error)
	RemoveProject(directory string) error
	UpdateQuota(directory, bhard string) error
}

func NewXfsQuotaer(xfsPath string) (IxfsQuotaer, error) {
	if _, err := os.Stat(xfsPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("xfs path %s does not exist", xfsPath)
	}

	isXfs, err := isXfs(xfsPath)
	if err != nil {
		return nil, fmt.Errorf("error checking if xfs path %s is an XFS filesystem: %v", xfsPath, err)
	}
	if !isXfs {
		return nil, fmt.Errorf("xfs path %s is not an XFS filesystem", xfsPath)
	}

	entry, err := getMountEntry(path.Clean(xfsPath), "xfs")
	if err != nil {
		return nil, err
	}
	if !strings.Contains(entry.VfsOpts, "pquota") && !strings.Contains(entry.VfsOpts, "prjquota") {
		return nil, fmt.Errorf("xfs path %s was not mounted with pquota nor prjquota", xfsPath)
	}

	_, err = exec.LookPath("xfs_quota")
	if err != nil {
		return nil, err
	}

	projectsFile := path.Join(xfsPath, "projects")
	directories := map[string]Info{}
	_, err = os.Stat(projectsFile)
	if os.IsNotExist(err) {
		file, cerr := os.Create(projectsFile)
		if cerr != nil {
			return nil, fmt.Errorf("error creating xfs projects file %s: %v", projectsFile, cerr)
		}
		file.Close()
	} else {
		directories, err = getExistingIDs(projectsFile)
		if err != nil {
			klog.Errorf("error while populating projectIDs map, there may be errors setting quotas later if projectIDs are reused: %v", err)
		}
	}

	xfsQuotaer := &xfsQuotaer{
		xfsPath:      xfsPath,
		projectsFile: projectsFile,
		directories:  directories,
		mapMutex:     &sync.Mutex{},
		fileMutex:    &sync.Mutex{},
	}

	err = xfsQuotaer.restoreQuotas()
	if err != nil {
		return nil, fmt.Errorf("error restoring quotas from projects file %s: %v", projectsFile, err)
	}

	return xfsQuotaer, nil
}

func isXfs(xfsPath string) (bool, error) {
	cmd := exec.Command("stat", "-f", "-c", "%T", xfsPath)
	out, err := cmd.Output()
	if err != nil {
		return false, err
	}
	if strings.TrimSpace(string(out)) != "xfs" {
		return false, nil
	}
	return true, nil
}

func getMountEntry(mountpoint, fstype string) (*mount.Info, error) {
	entries, err := mount.GetMounts()
	if err != nil {
		return nil, err
	}
	for _, e := range entries {
		if e.Mountpoint == mountpoint && e.Fstype == fstype {
			return e, nil
		}
	}
	return nil, fmt.Errorf("mount entry for mountpoint %s, fstype %s not found", mountpoint, fstype)
}

func (q *xfsQuotaer) restoreQuotas() error {
	for directory, info := range q.directories {
		//projectID, _ := strconv.ParseUint(string(match[1]), 10, 16)

		// If directory referenced by projects file no longer exists, don't set a
		// quota for it: will fail
		if _, err := os.Stat(directory); os.IsNotExist(err) {
			q.RemoveProject(directory)
			continue
		}

		if err := q.UpdateQuota(directory, info.Cap); err != nil {
			return fmt.Errorf("error restoring quota for directory %s: %v", directory, err)
		}
	}

	return q.dump()
}

func (q *xfsQuotaer) AddProject(directory, bhard string) (string, uint16, error) {
	projectID := generateID(q.mapMutex, q.directories)
	projectIDStr := strconv.FormatUint(uint64(projectID), 10)

	// Store project:directory mapping and also project's quota info
	block := "\n" + projectIDStr + ":" + directory + ":" + bhard + "\n"

	q.directories[directory] = Info{
		ID:  projectID,
		Cap: bhard,
	}

	// Specify the new project
	cmd := exec.Command("xfs_quota", "-x", "-c", fmt.Sprintf("project -s -p %s %s", directory, projectIDStr), q.xfsPath)
	out, err := cmd.CombinedOutput()
	if err != nil {
		delete(q.directories, directory)
		return "", 0, fmt.Errorf("xfs_quota failed with error: %v, output: %s", err, out)
	}
	q.UpdateQuota(directory, bhard)
	return block, projectID, q.dump()
}

func (q *xfsQuotaer) RemoveProject(directory string) error {
	deleteDirectory(q.mapMutex, q.directories, directory)
	return q.dump()
}

func (q *xfsQuotaer) UpdateQuota(directory, bhard string) error {
	v, ok := q.directories[directory]
	if !ok {
		return fmt.Errorf("directory with %v has not been added", directory)
	}
	projectIDStr := strconv.FormatUint(uint64(v.ID), 10)

	cmd := exec.Command("xfs_quota", "-x", "-c", fmt.Sprintf("limit -p bhard=%s %s", bhard, projectIDStr), q.xfsPath)

	q.directories[directory] = Info{
		ID:  v.ID,
		Cap: bhard,
	}

	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("xfs_quota failed with error: %v, output: %s", err, out)
	}

	return q.dump()
}

func (q *xfsQuotaer) dump() error {
	q.mapMutex.Lock()
	defer q.mapMutex.Unlock()
	return ioutil.WriteFile(q.projectsFile, []byte(q.directories.String()), 0)
}

type emptyQuota struct{}

func (e emptyQuota) AddProject(directory, bhard string) (string, uint16, error) {
	return "", 0, nil
}

func (e emptyQuota) RemoveProject(directory string) error {
	return nil
}

func (e emptyQuota) UpdateQuota(directory, bhard string) error {
	return nil
}

var DefaultQuotaer = emptyQuota{}
var _ IxfsQuotaer = emptyQuota{}
