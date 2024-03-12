package statedb

import (
	"fmt"
	"sort"

	sdk "github.com/cosmos/cosmos-sdk/types"
)

// SnapshotCommitCtx provides a way to create snapshots of branched contexts and
// only write state to the initial context when Commit() is called.
type SnapshotCommitCtx struct {
	initialCtx sdk.Context
	snapshots  []CtxSnapshot

	// always incrementing snapshot ID, used to identify snapshots.
	nextSnapshotID int
}

// NewSnapshotCtx creates a new SnapshotCtx from the initial context.
func NewSnapshotCtx(initialCtx sdk.Context) *SnapshotCommitCtx {
	sCtx := &SnapshotCommitCtx{
		initialCtx: initialCtx,
		snapshots:  nil,
		// Starts at -1 so the first snapshot is 0
		nextSnapshotID: -1,
	}

	// Create an initial snapshot of the initialCtx so no state is written until
	// Commit() is called. The ID is -1 but disregarded along with the
	// StoreRevertKey indices as this is only to branch the ctx.
	_ = sCtx.Snapshot(0, StoreRevertKey{0, 0, 0, 0})

	return sCtx
}

// CurrentCtx returns the current ctx, either the latest branched ctx, or the
// initial ctx if there are no snapshots.
func (c *SnapshotCommitCtx) CurrentCtx() sdk.Context {
	if len(c.snapshots) == 0 {
		return c.initialCtx
	}

	return c.snapshots[len(c.snapshots)-1].ctx
}

// CurrentSnapshot returns the current snapshot and true if there is one, or
// false if there are no snapshots.
func (c *SnapshotCommitCtx) CurrentSnapshot() (CtxSnapshot, bool) {
	if len(c.snapshots) == 0 {
		return CtxSnapshot{}, false
	}

	return c.snapshots[len(c.snapshots)-1], true
}

// Snapshot creates a new branched context and returns the revision id.
func (c *SnapshotCommitCtx) Snapshot(
	journalIndex int,
	storeRevertKey StoreRevertKey,
) int {
	id := c.nextSnapshotID
	c.nextSnapshotID++

	// Branch off a new CacheMultiStore + write function
	newCtx, newWrite := c.CurrentCtx().CacheContext()

	// Disable tracing for the branched context
	ms := newCtx.MultiStore().SetTracer(nil)
	newCtx = newCtx.WithMultiStore(ms)

	// Save the new snapshot to the list
	c.snapshots = append(c.snapshots, CtxSnapshot{
		id:    id,
		ctx:   newCtx,
		write: newWrite,

		journalIndex:   journalIndex,
		storeRevertKey: storeRevertKey,
	})

	return id
}

// Revert reverts the state to the given revision id.
func (c *SnapshotCommitCtx) Revert(revid int) {
	// Find the snapshot in the stack of valid snapshots.
	idx := sort.Search(len(c.snapshots), func(i int) bool {
		return c.snapshots[i].id >= revid
	})

	if idx == -1 {
		panic(fmt.Errorf("revision id %v does not exist", revid))
	}

	// Index is invalid or the revision id is not the same somehow
	if idx >= len(c.snapshots) || c.snapshots[idx].id != revid {
		panic(fmt.Errorf("revision id %v is invalid", revid))
	}

	// Remove invalidated snapshots
	c.snapshots = c.snapshots[:idx]
}

// Commit writes all the branched contexts to the initialContext.
func (c *SnapshotCommitCtx) Commit() {
	// Write snapshots from newest to oldest.
	// Each store.Write() applies the state changes to its parent / previous snapshot
	for i := len(c.snapshots) - 1; i >= 0; i-- {
		snapshot := c.snapshots[i]
		snapshot.write()

		// initialCtx should not be considered as a snapshot, so we don't need
		// to call Write() on it to apply the changes.
	}
}

// CtxSnapshot is a single snapshot with a branched context.
type CtxSnapshot struct {
	id    int
	ctx   sdk.Context
	write func()

	// In-memory things like logs, access list
	journalIndex   int
	storeRevertKey StoreRevertKey
}
