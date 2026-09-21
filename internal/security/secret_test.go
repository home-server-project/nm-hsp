// SPDX-License-Identifier: Apache-2.0

package security

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

func TestSecretFormattingIsAlwaysRedacted(t *testing.T) {
	secret := NewSecret("correct-horse")
	defer secret.Clear()

	for _, format := range []string{"%s", "%v", "%+v", "%#v", "%q"} {
		output := fmt.Sprintf(format, secret)
		if strings.Contains(output, "correct-horse") {
			t.Fatalf("format %q exposed secret: %q", format, output)
		}
		if !strings.Contains(output, redacted) {
			t.Fatalf("format %q did not render redaction marker: %q", format, output)
		}
	}
}

func TestSecretClearOverwritesBackingBytes(t *testing.T) {
	secret := NewSecret("correct-horse")
	backing := secret.value
	secret.Clear()

	if !secret.Empty() {
		t.Fatal("Clear() should empty the secret")
	}
	for i, b := range backing {
		if b != 0 {
			t.Fatalf("backing byte %d was not overwritten", i)
		}
	}
}

func TestRedactErrorDoesNotLeakSecretOrUnwrapOriginal(t *testing.T) {
	secret := NewSecret("correct-horse")
	defer secret.Clear()
	original := errors.New("D-Bus rejected psk correct-horse")

	redactedErr := RedactError(original, secret)
	if strings.Contains(redactedErr.Error(), "correct-horse") {
		t.Fatalf("redacted error leaked secret: %q", redactedErr)
	}
	if !strings.Contains(redactedErr.Error(), redacted) {
		t.Fatalf("redacted error missing marker: %q", redactedErr)
	}
	if errors.Unwrap(redactedErr) != nil {
		t.Fatal("redacted error must not expose the original error through Unwrap")
	}
}

func TestSecretIsHex(t *testing.T) {
	valid := NewSecret("0123456789abcdefABCDEF")
	defer valid.Clear()
	if !valid.IsHex() {
		t.Fatal("expected hexadecimal secret")
	}

	invalid := NewSecret("not-hex")
	defer invalid.Clear()
	if invalid.IsHex() {
		t.Fatal("non-hexadecimal secret should fail")
	}
}
