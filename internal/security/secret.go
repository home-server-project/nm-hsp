// SPDX-License-Identifier: Apache-2.0

package security

import (
	"errors"
	"fmt"
	"io"
	"strings"
)

const redacted = "[REDACTED]"

type secretState struct {
	value []byte
}

// Secret holds transient secret bytes without exposing them through the usual
// fmt/Stringer paths. Copies share one mutable state so clearing any copy makes
// every copy empty.
type Secret struct {
	state *secretState
}

// NewSecret copies a transient string into a Secret. Callers should release or
// overwrite their original string reference as soon as practical.
func NewSecret(value string) Secret {
	if value == "" {
		return Secret{}
	}
	return Secret{state: &secretState{value: []byte(value)}}
}

func (s Secret) bytes() []byte {
	if s.state == nil {
		return nil
	}
	return s.state.value
}

// Empty reports whether the secret currently contains no bytes.
func (s Secret) Empty() bool {
	return len(s.bytes()) == 0
}

// Len returns the number of bytes in the secret without revealing them.
func (s Secret) Len() int {
	return len(s.bytes())
}

// Value returns the secret as a string for an API that requires a string.
// The returned string is transient and cannot be reliably wiped by Go.
func (s Secret) Value() string {
	return string(s.bytes())
}

// IsHex reports whether every byte in a non-empty secret is hexadecimal.
func (s Secret) IsHex() bool {
	value := s.bytes()
	if len(value) == 0 {
		return false
	}
	for _, b := range value {
		if !((b >= '0' && b <= '9') || (b >= 'a' && b <= 'f') || (b >= 'A' && b <= 'F')) {
			return false
		}
	}
	return true
}

// Clear overwrites the shared mutable backing bytes and drops this reference.
// Other copies observe the same cleared state.
func (s *Secret) Clear() {
	if s == nil || s.state == nil {
		return
	}
	for i := range s.state.value {
		s.state.value[i] = 0
	}
	s.state.value = nil
	s.state = nil
}

// String deliberately never reveals the secret.
func (s Secret) String() string {
	return redacted
}

// GoString deliberately never reveals the secret in %#v formatting.
func (s Secret) GoString() string {
	return redacted
}

// Format redacts the secret for every fmt formatting verb.
func (s Secret) Format(state fmt.State, _ rune) {
	_, _ = io.WriteString(state, redacted)
}

// Redact removes known secret values from arbitrary text.
func Redact(text string, secrets ...Secret) string {
	for _, secret := range secrets {
		if secret.Empty() {
			continue
		}
		text = strings.ReplaceAll(text, secret.Value(), redacted)
	}
	return text
}

// RedactError creates a terminal sanitized error. It intentionally does not
// wrap the original error, so callers cannot accidentally unwrap and print an
// unredacted message later.
func RedactError(err error, secrets ...Secret) error {
	if err == nil {
		return nil
	}
	return errors.New(Redact(err.Error(), secrets...))
}
