// Package keychain stores API keys in the operating system credential store.
package keychain

import (
	"errors"
	"fmt"

	"github.com/zalando/go-keyring"
)

const service = "chattu"

// Set stores secret under the given account (provider ID).
func Set(account, secret string) error {
	if account == "" {
		return errors.New("keychain: account is required")
	}
	if secret == "" {
		return errors.New("keychain: secret is required")
	}
	if err := keyring.Set(service, account, secret); err != nil {
		return fmt.Errorf("keychain: set %q: %w", account, err)
	}
	return nil
}

// Get returns the stored secret for account.
func Get(account string) (string, error) {
	secret, err := keyring.Get(service, account)
	if err != nil {
		return "", fmt.Errorf("keychain: get %q: %w", account, err)
	}
	return secret, nil
}

// Delete removes any stored secret for account.
func Delete(account string) error {
	if err := keyring.Delete(service, account); err != nil && !errors.Is(err, keyring.ErrNotFound) {
		return fmt.Errorf("keychain: delete %q: %w", account, err)
	}
	return nil
}

// Has reports whether a secret exists for account.
func Has(account string) (bool, error) {
	_, err := keyring.Get(service, account)
	if errors.Is(err, keyring.ErrNotFound) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("keychain: lookup %q: %w", account, err)
	}
	return true, nil
}
