package main
import (
	"fmt"
	"os"

	"golang.org/x/term"
)

func GetPasswordStdin(prompt string) (string, error) {
	fmt.Printf("%s: ", prompt)
	b, err := term.ReadPassword(int(os.Stdin.Fd()))
	if err != nil {
		return "", err
	}
	fmt.Println()
	return string(b), nil
}
