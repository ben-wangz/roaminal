package monitor

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
)

type cgroup struct {
	mountpoint string
	relative   string
	read       readFile
}

func discoverCgroup(read readFile) (*cgroup, error) {
	data, err := read("/proc/self/cgroup")
	if err != nil {
		return nil, err
	}
	path, err := unifiedPath(string(data))
	if err != nil {
		return nil, err
	}
	mounts, err := read("/proc/self/mountinfo")
	if err != nil {
		return nil, err
	}
	root, mountpoint, err := unifiedMount(string(mounts))
	if err != nil {
		return nil, err
	}
	relative, err := relativeCgroupPath(path, root)
	if err != nil {
		return nil, err
	}
	return &cgroup{mountpoint: mountpoint, relative: relative, read: read}, nil
}

func (c *cgroup) file(name string) ([]byte, error) {
	base := filepath.Clean(filepath.Join(c.mountpoint, c.relative))
	target := filepath.Clean(filepath.Join(base, name))
	rel, err := filepath.Rel(c.mountpoint, target)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return nil, errors.New("cgroup path escapes mount")
	}
	return c.read(target)
}

func unifiedPath(data string) (string, error) {
	for _, line := range strings.Split(data, "\n") {
		fields := strings.SplitN(line, ":", 3)
		if len(fields) == 3 && fields[1] == "" {
			if strings.HasPrefix(fields[2], "/") {
				return fields[2], nil
			}
			return "/", nil
		}
	}
	return "", errors.New("unified cgroup entry not found")
}

func unifiedMount(data string) (string, string, error) {
	for _, line := range strings.Split(data, "\n") {
		parts := strings.SplitN(line, " - ", 2)
		if len(parts) != 2 {
			continue
		}
		post := strings.Fields(parts[1])
		if len(post) == 0 || post[0] != "cgroup2" {
			continue
		}
		pre := strings.Fields(parts[0])
		if len(pre) < 6 {
			continue
		}
		return unescapeMountInfo(pre[3]), unescapeMountInfo(pre[4]), nil
	}
	return "", "", errors.New("cgroup2 mount not found")
}

func relativeCgroupPath(path, root string) (string, error) {
	path, root = filepath.Clean(path), filepath.Clean(root)
	if root == "." {
		root = "/"
	}
	if root == "/" {
		return strings.TrimPrefix(path, "/"), nil
	}
	if path != root && !strings.HasPrefix(path, root+string(filepath.Separator)) {
		return "", fmt.Errorf("cgroup path %q is outside mount root %q", path, root)
	}
	return strings.TrimPrefix(strings.TrimPrefix(path, root), string(filepath.Separator)), nil
}

func unescapeMountInfo(value string) string {
	var out strings.Builder
	for i := 0; i < len(value); i++ {
		if i+3 < len(value) && value[i] == '\\' {
			if value[i+1] >= '0' && value[i+1] <= '7' && value[i+2] >= '0' && value[i+2] <= '7' && value[i+3] >= '0' && value[i+3] <= '7' {
				out.WriteByte((value[i+1]-'0')*64 + (value[i+2]-'0')*8 + value[i+3] - '0')
				i += 3
				continue
			}
		}
		out.WriteByte(value[i])
	}
	return out.String()
}
