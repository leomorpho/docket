package cmd

import (
	"fmt"

	"github.com/leomorpho/docket/internal/security"
)

func keystoreProvider() (security.KeystoreProvider, error) {
	if err := ensureDocketHome(); err != nil {
		return nil, fmt.Errorf("resolving DOCKET_HOME for keystore: %w", err)
	}
	return security.NewFileKeystore(docketHome), nil
}
