package main

import (
	"io/ioutil"
	"log/slog"
	"os"
	"path"
)

const fileName = ".doppler-inject.key"

var tmpDir = os.TempDir()
var fullFilePath = path.Join(tmpDir, fileName)

type InsecureFileStorage struct {
}

func (s *InsecureFileStorage) HasCachedSecrets() bool {
	_, err := os.Stat(fullFilePath)
	return !os.IsNotExist(err)
}

func (s *InsecureFileStorage) GetCachedSecrets() (*Secrets, error) {
	slog.Debug("Reading cached secrets from %s", fullFilePath)

	fileInfo, err := os.Stat(fullFilePath)
	if os.IsNotExist(err) || fileInfo.Size() == 0 {
		return nil, err
	}

	content, err := ioutil.ReadFile(fullFilePath)
	if err != nil {
		return nil, err
	}
	slog.Debug("Read cached secrets: %s", content)

	return Deserialize(content)
}

func (s *InsecureFileStorage) CacheSecrets(secrets *Secrets) error {
	err := MkdirRecursive(tmpDir)
	if err != nil {
		return err
	}

	file, err := CreateFileIfNotExist(fullFilePath)
	if err != nil {
		return err
	}
	defer file.Close()

	slog.Debug("Caching secrets to %s", fullFilePath)
	slog.Debug("Secrets: %s", secrets)
	serializedSecrets, err := secrets.Serialize()
	if err != nil {
		return err
	}

	_, err = file.Write(serializedSecrets)
	if err != nil {
		return err
	}

	return nil
}

func (s *InsecureFileStorage) CleanCachedSecrets() error {
	err := os.Remove(fullFilePath)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	slog.Debug("Removed cached secrets from %s", fullFilePath)
	return nil
}

func NewInsecureFileStorage() *InsecureFileStorage {
	return &InsecureFileStorage{}
}
