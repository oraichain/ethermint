package statedb_test

import (
	"testing"

	"github.com/evmos/ethermint/x/evm/statedb"
	"github.com/stretchr/testify/require"
)

func TestSnapshotCommitCtx(t *testing.T) {
	initialCtx := NewTestContext()
	snapshotCtx := statedb.NewSnapshotCtx(initialCtx)

	// Write to the snapshot
	store := snapshotCtx.CurrentCtx().KVStore(testKvStoreKey)

	key := []byte("key")
	value := []byte("value")
	store.Set(key, value)

	require.Equal(t, value, store.Get(key), "store should have value")

	// Fetch from initial context
	parentStore := initialCtx.KVStore(testKvStoreKey)
	require.Nil(t, parentStore.Get(key), "parent store should not have value")

	// Make a snapshot
	previousCtx := snapshotCtx.CurrentCtx()
	snapshotID := snapshotCtx.Snapshot(0)
	require.Equal(t, 1, snapshotID, "snapshot id should be 1")
	require.NotEqual(t, previousCtx, snapshotCtx.CurrentCtx(), "CurrentCtx should be branched")

	// Write to the snapshot
	store = snapshotCtx.CurrentCtx().KVStore(testKvStoreKey)
	key2 := []byte("key2")
	value2 := []byte("value2")
	store.Set(key2, value2)

	require.Equal(t, value2, store.Get(key2), "store should have value")
	require.Nil(t, parentStore.Get(key2), "parent store should not have value")

	// Commit snapshots
	snapshotCtx.Commit()

	// Fetch from initial context
	require.Equal(t, value, parentStore.Get(key), "parent store should have value")
	require.Equal(t, value2, parentStore.Get(key2), "parent store should have value from snapshot")
}

func TestRevert(t *testing.T) {
	initialCtx := NewTestContext()
	snapshotCtx := statedb.NewSnapshotCtx(initialCtx)

	// Make a snapshot
	snapshotID := snapshotCtx.Snapshot(0)

	// Revert to the snapshot
	snapshotCtx.Revert(snapshotID)

	// Try to revert to the same snapshot again, which should panic
	require.Panics(t, func() {
		snapshotCtx.Revert(snapshotID)
	}, "Reverting to the same snapshot should panic")

	// Try to revert to a non-existent snapshot, which should also panic
	require.Panics(t, func() {
		snapshotCtx.Revert(999)
	}, "Reverting to a non-existent snapshot should panic")
}
