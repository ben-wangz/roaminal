package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/ben-wangz/roaminal/agents/hooks/codex/internal/buildinfo"
)

type target struct{ Name, Path string }
type manifest struct {
	BundleSchemaVersion int            `json:"bundleSchemaVersion"`
	ComponentVersion    string         `json:"componentVersion"`
	BinaryName          string         `json:"binaryName"`
	HooksConfig         fileDigest     `json:"hooksConfig"`
	Targets             []targetDigest `json:"targets"`
}
type fileDigest struct {
	Path   string `json:"path"`
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
}
type targetDigest struct {
	Target string `json:"target"`
	Path   string `json:"path"`
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
}

func main() {
	root := flag.String("root", ".", "bundle root")
	output := flag.String("output", "manifest.json", "manifest path")
	flag.Parse()
	if err := run(*root, *output); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(root, output string) error {
	componentData, err := os.ReadFile(filepath.Join(root, "component.json"))
	if err != nil {
		return err
	}
	var component struct {
		BundleSchemaVersion int    `json:"bundleSchemaVersion"`
		ComponentVersion    string `json:"componentVersion"`
		BinaryName          string `json:"binaryName"`
	}
	if err := json.Unmarshal(componentData, &component); err != nil {
		return err
	}
	if component.BundleSchemaVersion < 1 || component.ComponentVersion == "" || component.BinaryName == "" {
		return errors.New("invalid component metadata")
	}
	configPath := filepath.Join(root, "config", "hooks.json")
	configDigest, err := digest(configPath, filepath.ToSlash(filepath.Join("config", "hooks.json")))
	if err != nil {
		return err
	}
	targets := []target{
		{"linux-amd64", filepath.Join(root, "linux-amd64", component.BinaryName)},
		{"linux-arm64", filepath.Join(root, "linux-arm64", component.BinaryName)},
		{"darwin-amd64", filepath.Join(root, "darwin-amd64", component.BinaryName)},
		{"darwin-arm64", filepath.Join(root, "darwin-arm64", component.BinaryName)},
	}
	sort.Slice(targets, func(i, j int) bool { return targets[i].Name < targets[j].Name })
	digests := make([]targetDigest, 0, len(targets))
	seen := map[string]bool{}
	for _, item := range targets {
		if seen[item.Name] {
			return errors.New("duplicate target")
		}
		seen[item.Name] = true
		value, err := digest(item.Path, filepath.ToSlash(filepath.Join(item.Name, component.BinaryName)))
		if err != nil {
			return err
		}
		digests = append(digests, targetDigest{Target: item.Name, Path: value.Path, Size: value.Size, SHA256: value.SHA256})
	}
	result := manifest{BundleSchemaVersion: component.BundleSchemaVersion, ComponentVersion: component.ComponentVersion, BinaryName: component.BinaryName, HooksConfig: configDigest, Targets: digests}
	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if err := os.WriteFile(output, data, 0644); err != nil {
		return err
	}
	_ = buildinfo.Version
	return nil
}

func digest(path, relative string) (fileDigest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return fileDigest{}, fmt.Errorf("read %s: %w", relative, err)
	}
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return fileDigest{}, fmt.Errorf("invalid file %s", relative)
	}
	digest := sha256.Sum256(data)
	return fileDigest{Path: relative, Size: info.Size(), SHA256: hex.EncodeToString(digest[:])}, nil
}
