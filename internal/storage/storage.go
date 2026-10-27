package storage

import (
	"encoding/json"
	"errors"
	"regexp"

	"github.com/napisani/secret_inject/internal/config"
	"github.com/napisani/secret_inject/internal/secret"
)

const cachePrefix = "sources-v1-"
const cacheVersionKey = "cache-version"

// Bump when stored snapshots or cache rules become incompatible.
const cacheVersion = "1\n"

var scopePattern = regexp.MustCompile("^[0-9a-f]{64}$")

type Storage interface {
	Initialize() error
	Read(scope string) (*secret.Secrets, bool, error)
	Write(scope string, secrets *secret.Secrets) error
	CleanAll() error
}

func cacheKey(scope string) (string, error) {
	if !scopePattern.MatchString(scope) {
		return "", errors.New("invalid cache scope")
	}
	return cachePrefix + scope, nil
}

func decodeSnapshot(data []byte) (*secret.Secrets, error) {
	var snapshot secret.Secrets
	if err := json.Unmarshal(data, &snapshot); err != nil {
		return nil, errors.New("invalid cached secret snapshot")
	}
	if snapshot.Entries == nil || snapshot.Timestamp.IsZero() {
		return nil, errors.New("invalid cached secret snapshot")
	}
	return &snapshot, nil
}

func Get(cfg *config.StorageConfig) (Storage, error) {
	if cfg == nil {
		return nil, errors.New("storage configuration required")
	}
	switch cfg.Type {
	case "keyring":
		return nil, ErrStoreUnavailable
	case "file":
		return NewFile()
	default:
		return nil, errors.New("unknown storage type")
	}
}
