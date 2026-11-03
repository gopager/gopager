package main

import (
	"errors"
	"strings"
	"testing"

	"github.com/gopager/gopager/internal/identity"
)

func TestBootstrapPasswordInputPreservesBytes(t *testing.T) {
	for _, test := range []struct {
		input string
		want  string
	}{
		{"initial-password\n", "initial-password"},
		{"initial-password", "initial-password"},
		{"  exact-password  \n", "  exact-password  "},
		{"password-with-CR\r\n", "password-with-CR\r"},
	} {
		actual, err := readBootstrapPassword(strings.NewReader(test.input))
		if err != nil || actual != test.want {
			t.Fatalf("password transport changed bytes: %v", err)
		}
	}
	for _, input := range []string{"", "short\n", strings.Repeat("x", 1025), strings.Repeat("x", 1026) + "\n"} {
		if _, err := readBootstrapPassword(strings.NewReader(input)); !errors.Is(err, identity.ErrInvalidInput) {
			t.Fatalf("invalid password input accepted: %v", err)
		}
	}
}
