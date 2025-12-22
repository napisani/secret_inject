package storage

import (
	"fmt"
	"os"

	"golang.org/x/term"
)

func getPasswordStdin(prompt string) (string, error) {
	fmt.Printf("%s: ", prompt)
	b, err := term.ReadPassword(int(os.Stdin.Fd()))
	if err != nil {
		return "", err
	}
	fmt.Println()
	return string(b), nil
}
