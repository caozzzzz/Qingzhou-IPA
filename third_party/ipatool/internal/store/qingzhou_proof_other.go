//go:build !ios

package store

import "context"

func platformMachineProof(_ context.Context, _, _, _ string) (map[string]string, error) {
	return nil, nil
}
