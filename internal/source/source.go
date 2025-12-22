package source

import "github.com/napisani/secret_inject/internal/secret"

type Source interface {
	Init(config map[string]interface{}) error
	GetAllSecrets() (*secret.Secrets, error)
	IsEnabled() bool
}
