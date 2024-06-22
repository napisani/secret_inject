package main

type Storage interface {
  HasCachedSecrets() bool
	GetCachedSecrets() (*Secrets, error)
	CacheSecrets(secrets *Secrets) error
	CleanCachedSecrets() error
}

func GetStorage() (Storage, error) {
	// v := NewInsecureFileStorage()
  return NewKeyringStorage()

	// return v
}
