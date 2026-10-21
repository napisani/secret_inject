package storage

import (
	"fmt"
	"os"

	"golang.org/x/term"
)

func getPasswordStdin(prompt string) (string, error) {
	if _, err := fmt.Fprintf(os.Stderr, "%s: ", prompt); err != nil {
		return "", fmt.Errorf("writing password prompt: %w", err)
	}
	b, err := term.ReadPassword(int(os.Stdin.Fd()))
	if err != nil {
		return "", fmt.Errorf("reading password: %w", err)
	}
	if _, err := fmt.Fprintln(os.Stderr); err != nil {
		return "", fmt.Errorf("writing password prompt newline: %w", err)
	}
	return string(b), nil
}
