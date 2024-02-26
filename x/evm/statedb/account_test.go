package statedb_test

import (
	"math/big"
	"testing"

	"github.com/evmos/ethermint/x/evm/statedb"
	"github.com/stretchr/testify/require"
)

func TestAccountIsEmpty(t *testing.T) {
	tests := []struct {
		name     string
		account  statedb.Account
		expected bool
	}{
		{
			name: "Empty account",
			account: statedb.Account{
				Balance:  big.NewInt(0),
				Nonce:    0,
				CodeHash: emptyCodeHash,
			},
			expected: true,
		},
		{
			name: "Non-empty balance",
			account: statedb.Account{
				Balance:  big.NewInt(1),
				Nonce:    0,
				CodeHash: emptyCodeHash,
			},
			expected: false,
		},
		{
			name: "Non-zero nonce",
			account: statedb.Account{
				Balance:  big.NewInt(0),
				Nonce:    1,
				CodeHash: emptyCodeHash,
			},
			expected: false,
		},
		{
			name: "Non-empty code hash",
			account: statedb.Account{
				Balance:  big.NewInt(0),
				Nonce:    0,
				CodeHash: []byte{1, 2, 3},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(
				t,
				tt.expected,
				tt.account.IsEmpty(),
			)
		})
	}
}
