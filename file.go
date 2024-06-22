package main

import (
	"log/slog"
	"os"
)

func MkdirRecursive(path string) error {
	slog.Debug("Creating directory (if does not exist): %s", path)
	err := os.MkdirAll(path, 0755)
	if err != nil {
		return err
	}
	slog.Debug("Created directory: %s", path)
	return nil
}

func CreateFileIfNotExist(filename string) (*os.File, error) {
	_, err := os.Stat(filename)
	if os.IsNotExist(err) {
		slog.Debug("Creating file: %s", filename)
		file, err := os.Create(filename)
		if err != nil {
			return file, err
		}
		return file, nil
	}

	slog.Debug("Opening file: %s", filename)
	return os.OpenFile(filename, os.O_RDWR, 0644)
}
