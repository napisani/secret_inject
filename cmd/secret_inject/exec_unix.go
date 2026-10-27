//go:build linux || darwin

package main

import "syscall"

func platformExec(path string, argv, environment []string) error {
	return syscall.Exec(path, argv, environment)
}
