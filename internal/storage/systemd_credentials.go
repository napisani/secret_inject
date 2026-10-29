package storage

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"time"

	"github.com/napisani/secret_inject/internal/privatefs"
)

const systemdValueLimit = 64 * 1024
const systemdCipherLimit = 2 * systemdValueLimit

// Nix binds the native helper into the package closure, independent of PATH.
var systemdCredsExecutable = "/usr/bin/systemd-creds"

type systemdCredentials struct {
	directory  string
	executable string
	hostKey    string
}

func openSystemdCredentials(ctx context.Context, directory string, policy StorePolicy) (NativeByteAdapter, error) {
	var executable string
	for _, path := range []string{systemdCredsExecutable, "/run/current-system/sw/bin/systemd-creds"} {
		info, err := os.Stat(path)
		if err == nil && info.Mode().IsRegular() && info.Mode().Perm()&0111 != 0 {
			executable = path
			break
		}
	}
	if executable == "" {
		return nil, ErrStoreUnavailable
	}
	native := &systemdCredentials{
		directory: directory, executable: executable,
		hostKey: "/var/lib/systemd/credential.secret",
	}
	if err := native.prepare(ctx, policy); err != nil {
		return nil, err
	}
	return native, nil
}

func (s *systemdCredentials) prepare(ctx context.Context, policy StorePolicy) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if policy == InteractiveSetup {
		if err := privatefs.EnsureDir(s.directory); err != nil {
			return ErrStoreUnavailable
		}
		// Only explicit preparation may initialize systemd's host encryption key.
		plain := []byte("secret-inject-native-readiness-v1")
		cipher, err := s.crypt(ctx, "encrypt", "secret-inject-readiness", plain)
		if err != nil {
			return err
		}
		decoded, err := s.crypt(ctx, "decrypt", "secret-inject-readiness", cipher)
		if err != nil {
			return err
		}
		if !bytes.Equal(decoded, plain) {
			return ErrStoreOperation
		}
		return nil
	}
	if policy != Unattended {
		return ErrStoreUnavailable
	}
	directory, err := privatefs.OpenDir(s.directory)
	if os.IsNotExist(err) {
		return ErrInteractionRequired
	}
	if err != nil {
		return ErrStoreUnavailable
	}
	if err := directory.Close(); err != nil {
		return ErrStoreOperation
	}
	return s.hostKeyReady()
}

func (s *systemdCredentials) hostKeyReady() error {
	info, err := os.Stat(s.hostKey)
	if os.IsNotExist(err) {
		return ErrInteractionRequired
	}
	if err != nil || !info.Mode().IsRegular() {
		return ErrStoreUnavailable
	}
	return nil
}

func (*systemdCredentials) MaxValueBytes() int { return systemdValueLimit }
func (*systemdCredentials) Close() error       { return nil }

func systemdObjectName(key string) (string, error) {
	return encryptedObjectName(key, ".cred")
}

func (s *systemdCredentials) files() *encryptedFiles {
	return &encryptedFiles{
		directory: s.directory, extension: ".cred",
		valueLimit: systemdValueLimit, cipherLimit: systemdCipherLimit,
		encrypt: func(ctx context.Context, name string, value []byte) ([]byte, error) {
			if err := s.hostKeyReady(); err != nil {
				return nil, err
			}
			return s.crypt(ctx, "encrypt", name, value)
		},
		decrypt: func(ctx context.Context, name string, value []byte) ([]byte, error) {
			return s.crypt(ctx, "decrypt", name, value)
		},
	}
}

func (s *systemdCredentials) Get(ctx context.Context, key string) ([]byte, bool, error) {
	return s.files().Get(ctx, key)
}

func (s *systemdCredentials) Put(ctx context.Context, key string, value []byte) error {
	return s.files().Put(ctx, key, value)
}

func (s *systemdCredentials) Delete(ctx context.Context, key string) error {
	return s.files().Delete(ctx, key)
}

func (s *systemdCredentials) Keys(ctx context.Context) ([]string, error) {
	return s.files().Keys(ctx)
}

func (s *systemdCredentials) crypt(ctx context.Context, operation, name string, input []byte) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	args := []string{"--user", "--no-ask-password", "--name=" + name}
	limit := systemdValueLimit
	if operation == "encrypt" {
		args = append(args, "--with-key=host")
		limit = systemdCipherLimit
	} else {
		args = append(args, "--refuse-null")
	}
	args = append(args, operation, "-", "-")
	command := exec.CommandContext(ctx, s.executable, args...)
	command.Env = []string{"LC_ALL=C", "SYSTEMD_LOG_LEVEL=err", "SYSTEMD_COLORS=0"}
	command.Stdin = bytes.NewReader(input)
	command.WaitDelay = time.Second
	var output systemdOutput
	output.limit = limit
	command.Stdout = &output
	command.Stderr = io.Discard
	if err := command.Run(); err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if errors.Is(err, os.ErrNotExist) {
			return nil, ErrStoreUnavailable
		}
		return nil, ErrStoreOperation
	}
	return output.buffer.Bytes(), nil
}

type systemdOutput struct {
	buffer bytes.Buffer
	limit  int
}

func (b *systemdOutput) Write(value []byte) (int, error) {
	if len(value) > b.limit-b.buffer.Len() {
		return 0, ErrStoreOperation
	}
	return b.buffer.Write(value)
}
