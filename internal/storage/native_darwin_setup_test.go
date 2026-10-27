//go:build darwin && cgo

package storage_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestKeychainPreparationCreatesAfterLazyOpen(t *testing.T) {
	t.Parallel()
	source, err := os.ReadFile("native_darwin.go")
	if err != nil {
		t.Fatal(err)
	}
	start := strings.Index(string(source), "static OSStatus si_open(")
	if start < 0 {
		t.Fatal("native open helper is missing")
	}
	end := strings.Index(string(source[start:]), "\nstatic OSStatus si_ready(")
	if end < 0 {
		t.Fatal("native open helper boundary is missing")
	}
	// Go test files cannot import C; compile the production helper directly.
	// Intercept creation so the probe cannot create or unlock any keychain.
	program := `#include <Security/Security.h>
#include <CoreFoundation/CoreFoundation.h>
#include <stdio.h>
static int creates = 0;
static OSStatus intercept_create(const char *path, UInt32 length, const void *password,
                                 Boolean prompt, SecAccessRef access, SecKeychainRef *keychain) {
    creates++;
    if (*keychain != NULL) return errSecDecode;
    return errSecUserCanceled;
}
#define SecKeychainCreate intercept_create
` + string(source[start:start+end]) + `
int main(int argc, char **argv) {
    if (argc != 2) return 2;
    Boolean previous;
    if (SecKeychainGetUserInteractionAllowed(&previous) != errSecSuccess) return 3;
    if (SecKeychainSetUserInteractionAllowed(false) != errSecSuccess) return 4;
    SecKeychainRef keychain = NULL;
    OSStatus setup = si_open(argv[1], 1, &keychain);
    if (keychain) CFRelease(keychain);
    if (setup != errSecUserCanceled || creates != 1) {
        fprintf(stderr, "missing-keychain setup did not reach creation: status=%d attempts=%d\n",
                (int)setup, creates);
        SecKeychainSetUserInteractionAllowed(previous);
        return 5;
    }
    keychain = NULL;
    OSStatus unattended = si_open(argv[1], 0, &keychain);
    if (keychain) CFRelease(keychain);
    SecKeychainSetUserInteractionAllowed(previous);
    if (unattended != errSecNoSuchKeychain || creates != 1) {
        fprintf(stderr, "unattended missing-keychain access attempted setup\n");
        return 6;
    }
    return 0;
}
`
	root := t.TempDir()
	fixture := filepath.Join(root, "probe.c")
	binary := filepath.Join(root, "probe")
	missing := filepath.Join(root, "missing.keychain")
	if err := os.WriteFile(fixture, []byte(program), 0600); err != nil {
		t.Fatal(err)
	}
	compiler, err := exec.LookPath("cc")
	if err != nil {
		t.Fatal("a C compiler is required for the native Keychain regression")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	build := exec.CommandContext(
		ctx,
		compiler,
		"-Wno-deprecated-declarations",
		"-framework", "Security",
		"-framework", "CoreFoundation",
		fixture, "-o", binary,
	)
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("native regression compilation failed: %v\n%s", err, output)
	}
	if output, err := exec.CommandContext(ctx, binary, missing).CombinedOutput(); err != nil {
		t.Fatalf("native preparation regression failed: %v\n%s", err, output)
	}
	for _, path := range []string{missing, missing + "-db"} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatal("the missing-keychain probe created a native store")
		}
	}
}
