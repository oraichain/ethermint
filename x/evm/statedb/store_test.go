package statedb_test

import (
	"testing"

	"github.com/ethereum/go-ethereum/common"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/evmos/ethermint/x/evm/statedb"
	"github.com/stretchr/testify/require"
)

func TestNewEphemeralStore_GetRevertKey(t *testing.T) {
	tests := []struct {
		name     string
		maleate  func(store *statedb.EphemeralStore)
		expected statedb.StoreRevertKey
	}{
		{
			"empty store",
			func(store *statedb.EphemeralStore) {},
			statedb.StoreRevertKey{},
		},
		{
			"store with refund",
			func(store *statedb.EphemeralStore) {
				store.AddRefund(1)
			},
			statedb.StoreRevertKey{
				RefundIndex:           1,
				SuicidedAccountsIndex: 0,
				LogsIndex:             0,
			},
		},
		{
			"store with suicided account",
			func(store *statedb.EphemeralStore) {
				store.SetAccountSuicided(address)
			},
			statedb.StoreRevertKey{
				RefundIndex:           0,
				SuicidedAccountsIndex: 1,
				LogsIndex:             0,
			},
		},
		{
			"store with log",
			func(store *statedb.EphemeralStore) {
				store.AddLog(&ethtypes.Log{})
			},
			statedb.StoreRevertKey{
				RefundIndex:           0,
				SuicidedAccountsIndex: 0,
				LogsIndex:             1,
			},
		},
		{
			"store with refund, suicided account and log",
			func(store *statedb.EphemeralStore) {
				store.AddRefund(1)
				store.SetAccountSuicided(address)
				store.AddLog(&ethtypes.Log{})
			},
			statedb.StoreRevertKey{
				RefundIndex:           1,
				SuicidedAccountsIndex: 1,
				LogsIndex:             1,
			},
		},
		{
			"store with multiple refund, suicided account and log",
			func(store *statedb.EphemeralStore) {
				store.AddRefund(1)
				store.AddRefund(2)
				store.AddRefund(3)
				store.SetAccountSuicided(address)
				store.SetAccountSuicided(address2)

				store.AddLog(&ethtypes.Log{})
				store.AddLog(&ethtypes.Log{})
			},
			statedb.StoreRevertKey{
				RefundIndex:           3,
				SuicidedAccountsIndex: 2,
				LogsIndex:             2,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := statedb.NewEphemeralStore()

			tt.maleate(store)

			key := store.GetRevertKey()
			require.Equal(t, tt.expected, key)
		})
	}
}

func TestEphemeralStore_Revert(t *testing.T) {
	tests := []struct {
		name          string
		init          func(store *statedb.EphemeralStore)
		afterSnapshot func(store *statedb.EphemeralStore)
		wantState     *statedb.EphemeralStore
	}{
		{
			"empty store",
			func(store *statedb.EphemeralStore) {},
			func(store *statedb.EphemeralStore) {},
			&statedb.EphemeralStore{
				RefundStates:          nil,
				SuicidedAccountStates: nil,
				Logs:                  nil,
			},
		},
		{
			"empty store snapshot - reverts",
			func(store *statedb.EphemeralStore) {},
			func(store *statedb.EphemeralStore) {
				store.AddRefund(1)
				store.SetAccountSuicided(address)
				store.AddLog(&ethtypes.Log{})
			},
			&statedb.EphemeralStore{
				RefundStates:          []uint64{},
				SuicidedAccountStates: []common.Address{},
				Logs:                  []*ethtypes.Log{},
			},
		},
		{
			"non-empty store snapshot - reverts",
			func(store *statedb.EphemeralStore) {
				store.AddRefund(1)
				store.AddRefund(3)
				store.AddRefund(10)
				store.SubRefund(2)
				store.SubRefund(1)

				store.SetAccountSuicided(address)
				store.SetAccountSuicided(address2)
				store.AddLog(&ethtypes.Log{
					Index: 1,
				})
				store.AddLog(&ethtypes.Log{
					Index: 2,
				})
				store.AddLog(&ethtypes.Log{
					Index: 3,
				})
			},
			func(store *statedb.EphemeralStore) {
				store.AddRefund(20)
				store.SubRefund(5)
				store.SetAccountSuicided(address3)
				store.AddLog(&ethtypes.Log{
					Index: 4,
				})
			},
			&statedb.EphemeralStore{
				// RefundStates get added up to the previous state
				RefundStates:          []uint64{1, 4, 14, 12, 11},
				SuicidedAccountStates: []common.Address{address, address2},
				Logs: []*ethtypes.Log{
					&ethtypes.Log{Index: 1},
					&ethtypes.Log{Index: 2},
					&ethtypes.Log{Index: 3},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := statedb.NewEphemeralStore()

			// Initialize the store
			tt.init(store)

			// Get the revert key before changing state
			revertKey := store.GetRevertKey()

			// Perform some operations on the store
			tt.afterSnapshot(store)

			// Revert the store to the previous state
			store.Revert(revertKey)

			// Verify that the store has been reverted correctly
			require.Equal(t, tt.wantState, store)
		})
	}
}

func TestEphemeralStore_Revert_InvalidKey(t *testing.T) {
	store := statedb.NewEphemeralStore()

	// Revert with an invalid key
	require.Panics(t, func() {
		store.Revert(statedb.StoreRevertKey{
			RefundIndex:           1,
			SuicidedAccountsIndex: 2,
			LogsIndex:             3,
		})
	}, "reverting with an invalid key should panic")
}

func TestEphemeralStore_GetLogs(t *testing.T) {
	store := statedb.NewEphemeralStore()

	// Add some logs
	store.AddLog(&ethtypes.Log{
		Index: 1,
	})
	store.AddLog(&ethtypes.Log{
		Index: 2,
	})
	store.AddLog(&ethtypes.Log{
		Index: 3,
	})

	// Get the logs
	logs := store.GetLogs()

	// Verify that the logs are correct
	require.Equal(t, []*ethtypes.Log{
		&ethtypes.Log{Index: 1},
		&ethtypes.Log{Index: 2},
		&ethtypes.Log{Index: 3},
	}, logs)

	require.Equal(t, len(logs), len(store.Logs))
}

func TestEphemeralStore_AddRefund(t *testing.T) {
	store := statedb.NewEphemeralStore()

	refunds := []uint64{1, 2, 3, 4, 5}
	refundSum := uint64(0)

	// Add some refunds
	for _, refund := range refunds {
		store.AddRefund(refund)
		refundSum += refund
	}

	// Get the current refund amount
	refund := store.GetRefund()

	require.Equal(t, refundSum, refund)
}

func TestEphemeralStore_SubRefund(t *testing.T) {
	store := statedb.NewEphemeralStore()

	refundAdds := []uint64{10, 20, 30, 40, 50}
	refundSubs := []uint64{1, 2, 3, 4, 5}

	refundSum := uint64(0)

	// Add some refunds
	for _, refund := range refundAdds {
		store.AddRefund(refund)
		refundSum += refund
	}

	// Subtract some refunds
	for _, refund := range refundSubs {
		store.SubRefund(refund)
		refundSum -= refund
	}

	// Get the current refund amount
	refund := store.GetRefund()

	require.Equal(t, refundSum, refund)
}

func TestEphemeralStore_AccountSuicided(t *testing.T) {
	store := statedb.NewEphemeralStore()

	// Set the account suicided
	store.SetAccountSuicided(address)

	// Verify that the account is suicided
	require.True(t, store.GetAccountSuicided(address))

	// No duplicate entries for the same account
	store.SetAccountSuicided(address)
	store.SetAccountSuicided(address)

	store.SetAccountSuicided(address2)

	require.Equal(
		t,
		[]common.Address{address, address2},
		store.SuicidedAccountStates,
		"repeated calls to SetAccountSuicided should not add duplicate entries",
	)

	// Remove
	allAddrs := store.GetAllSuicided()

	require.Equal(t, []common.Address{address, address2}, allAddrs)
}
