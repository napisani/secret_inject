//go:build nativeconformance || providerconformance

package conformance

import (
	"errors"
	"os"
	"os/user"
	"strings"
)

func RequireDisposableAccount() error {
	account, err := user.Current()
	if err != nil || os.Getenv("SECRET_INJECT_CONFORMANCE_AUTHORIZED") != "disposable-account-only" {
		return errors.New("conformance requires explicit disposable-account authorization")
	}
	name := account.Username
	if !strings.HasPrefix(name, "secret-inject-conformance") {
		return errors.New("conformance must run as a dedicated secret-inject-conformance account")
	}
	return nil
}
