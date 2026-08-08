package sshkey

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/ben-wangz/roaminal/backend/internal/sshfs"
)

type GenerationRequest struct {
	Algorithm string `json:"algorithm"`
	RSABits   *int   `json:"rsaBits"`
	FileName  string `json:"fileName"`
	Comment   string `json:"comment"`
}

type GenerationPaths struct {
	Algorithm        string
	StagingDirectory string
	PrivateStaging   string
	PublicStaging    string
	PrivateName      string
	PublicName       string
}

func (i *Inventory) PrepareGeneration(instanceID string, request GenerationRequest) (GenerationPaths, error) {
	if i == nil || i.Root == nil {
		return GenerationPaths{}, sshfs.ErrUnavailable
	}
	if i.KeygenPath == "" {
		return GenerationPaths{}, errors.New("ssh-keygen unavailable")
	}
	if err := i.Root.EnsureDirectory(); err != nil {
		return GenerationPaths{}, err
	}
	algorithm := strings.ToLower(strings.TrimSpace(request.Algorithm))
	if algorithm != "ed25519" && algorithm != "rsa" {
		return GenerationPaths{}, errors.New("unsupported key algorithm")
	}
	i.generationMu.Lock()
	if i.generating == nil {
		i.generating = make(map[string]struct{})
	}
	for _, key := range i.List() {
		if key.Algorithm == algorithm {
			i.generationMu.Unlock()
			return GenerationPaths{}, fmt.Errorf("%s key already exists", algorithm)
		}
	}
	if _, ok := i.generating[algorithm]; ok {
		i.generationMu.Unlock()
		return GenerationPaths{}, fmt.Errorf("%s key generation is already in progress", algorithm)
	}
	i.generating[algorithm] = struct{}{}
	i.generationMu.Unlock()
	reserved := true
	defer func() {
		if reserved {
			i.releaseGeneration(algorithm)
		}
	}()
	if request.FileName == "" || strings.ContainsAny(request.FileName, `/\\`) || strings.HasSuffix(request.FileName, ".pub") {
		return GenerationPaths{}, errors.New("invalid key filename")
	}
	nameAlgorithm, ok := AlgorithmForName(request.FileName)
	if !ok || nameAlgorithm != algorithm {
		return GenerationPaths{}, errors.New("key filename does not match algorithm")
	}
	bits := 0
	if request.RSABits != nil {
		bits = *request.RSABits
	}
	if algorithm == "rsa" {
		if bits == 0 {
			bits = 3072
		}
		if bits != 2048 && bits != 3072 && bits != 4096 {
			return GenerationPaths{}, errors.New("rsaBits must be 2048, 3072, or 4096")
		}
	} else if bits != 0 {
		return GenerationPaths{}, errors.New("rsaBits is only valid for rsa")
	}
	if !utf8.ValidString(request.Comment) || len([]byte(request.Comment)) > 255 {
		return GenerationPaths{}, errors.New("comment exceeds 255 bytes")
	}
	for _, value := range request.Comment {
		if unicode.IsControl(value) {
			return GenerationPaths{}, errors.New("comment contains a control character")
		}
	}
	publicName := request.FileName + ".pub"
	for _, name := range []string{request.FileName, publicName} {
		if info, err := i.Root.Lstat(name); err == nil {
			if info.Mode()&os.ModeSymlink != 0 {
				return GenerationPaths{}, errors.New("key target is a symlink")
			}
			return GenerationPaths{}, errors.New("key target already exists")
		} else if !errors.Is(err, os.ErrNotExist) {
			return GenerationPaths{}, err
		}
		if writable, reason := i.Root.CanWrite(name); !writable {
			return GenerationPaths{}, fmt.Errorf("key target is not writable: %s", reason)
		}
	}
	if instanceID == "" || strings.ContainsAny(instanceID, `/\\`) {
		return GenerationPaths{}, errors.New("invalid generation instance id")
	}
	staging := ".roaminal-keygen-" + instanceID
	if err := i.Root.MkdirAll(staging, 0o700); err != nil {
		return GenerationPaths{}, err
	}
	if err := i.Root.Chmod(staging, 0o700); err != nil {
		return GenerationPaths{}, err
	}
	reserved = false
	return GenerationPaths{Algorithm: algorithm, StagingDirectory: staging, PrivateStaging: staging + "/private", PublicStaging: staging + "/private.pub", PrivateName: request.FileName, PublicName: publicName}, nil
}

func (i *Inventory) GenerationCommand(paths GenerationPaths, request GenerationRequest) []string {
	privatePath := paths.PrivateStaging
	if i != nil && i.Root != nil && i.Root.Name() != "" {
		privatePath = filepath.Join(i.Root.Name(), privatePath)
	}
	argv := []string{i.KeygenPath, "-t", paths.Algorithm, "-f", privatePath}
	if paths.Algorithm == "rsa" {
		bits := 3072
		if request.RSABits != nil && *request.RSABits != 0 {
			bits = *request.RSABits
		}
		argv = append(argv, "-b", fmt.Sprintf("%d", bits))
	}
	if request.Comment != "" {
		argv = append(argv, "-C", request.Comment)
	}
	return argv
}

func (i *Inventory) Promote(paths GenerationPaths) error {
	defer i.releaseGeneration(paths.Algorithm)
	if i == nil || i.Root == nil || !i.Root.Available() {
		return sshfs.ErrUnavailable
	}
	data, _, err := i.Root.ReadFile(paths.PublicStaging, sshfs.PublicKeyMaxBytes)
	if err != nil {
		return fmt.Errorf("generated public key is unavailable: %w", err)
	}
	if !validPublicAlgorithm(data, paths.Algorithm) {
		return errors.New("generated public key is invalid")
	}
	private, err := i.Root.Lstat(paths.PrivateStaging)
	if err != nil {
		return fmt.Errorf("generated private key is unavailable: %w", err)
	}
	if private.Mode()&os.ModeSymlink != 0 || !private.Mode().IsRegular() {
		return errors.New("generated private key is not a regular file")
	}
	if err := i.Root.PromoteNoReplace(paths.PublicStaging, paths.PublicName); err != nil {
		return fmt.Errorf("publish public key: %w", err)
	}
	if err := i.Root.PromoteNoReplace(paths.PrivateStaging, paths.PrivateName); err != nil {
		_ = i.Root.Remove(paths.PublicName)
		return fmt.Errorf("publish private key: %w", err)
	}
	_ = i.Root.RemoveAll(paths.StagingDirectory)
	return nil
}

func (i *Inventory) DiscardGeneration(paths GenerationPaths) error {
	defer i.releaseGeneration(paths.Algorithm)
	if i == nil || i.Root == nil || paths.StagingDirectory == "" {
		return nil
	}
	return i.Root.RemoveAll(paths.StagingDirectory)
}

func (i *Inventory) CleanupStaging() {
	if i == nil || i.Root == nil || !i.Root.Available() {
		return
	}
	entries, err := i.Root.ReadDir()
	if err != nil {
		return
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".roaminal-keygen-") {
			_ = i.Root.RemoveAll(entry.Name())
		}
	}
}

func validPublicAlgorithm(data []byte, algorithm string) bool {
	fields := strings.Fields(strings.SplitN(string(data), "\n", 2)[0])
	if len(fields) < 2 {
		return false
	}
	return (algorithm == "ed25519" && fields[0] == "ssh-ed25519") || (algorithm == "rsa" && fields[0] == "ssh-rsa")
}
