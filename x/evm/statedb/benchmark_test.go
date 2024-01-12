package statedb_test

import (
	"math/big"
	"testing"

	"github.com/cosmos/cosmos-sdk/store"
	storetypes "github.com/cosmos/cosmos-sdk/store/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/ethereum/go-ethereum/common"
	"github.com/evmos/ethermint/x/evm/statedb"
	"github.com/tendermint/tendermint/libs/log"
	tmproto "github.com/tendermint/tendermint/proto/tendermint/types"
	dbm "github.com/tendermint/tm-db"
)

func NewTestContext() sdk.Context {
	storeKey := storetypes.NewKVStoreKey("test")

	db := dbm.NewMemDB()
	cms := store.NewCommitMultiStore(db)

	cms.MountStoreWithDB(storeKey, storetypes.StoreTypeIAVL, db)
	if err := cms.LoadLatestVersion(); err != nil {
		panic(err)
	}

	return sdk.NewContext(cms, tmproto.Header{}, false, log.NewNopLogger())
}

func benchmarkNestedSnapshot(b *testing.B, layers int) {
	db := statedb.New(NewTestContext(), NewMockKeeper(), emptyTxConfig)

	for i := 0; i < layers; i++ {
		db.Snapshot()

		key := common.BigToHash(big.NewInt(int64(i)))
		value := common.BigToHash(big.NewInt(int64(i + 1)))
		db.SetState(address, key, value)
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		db.ForEachStorage(address, func(k, v common.Hash) bool {
			return true
		})
	}
}

func BenchmarkNestedSnapshot1(b *testing.B) {
	benchmarkNestedSnapshot(b, 1)
}

func BenchmarkNestedSnapshot4(b *testing.B) {
	benchmarkNestedSnapshot(b, 4)
}

func BenchmarkNestedSnapshot8(b *testing.B) {
	benchmarkNestedSnapshot(b, 8)
}

func BenchmarkNestedSnapshot16(b *testing.B) {
	benchmarkNestedSnapshot(b, 16)
}

func benchmarkRevertToSnapshot(b *testing.B, layers int) {
	for i := 0; i < b.N; i++ {
		// Stop timer for setup -- can't be done before loop since we need to
		// reset the database after each revert
		// TODO: This takes way too long since RevertToSnapshot is really quick
		// compared to the setup
		b.StopTimer()
		db := statedb.New(NewTestContext(), NewMockKeeper(), emptyTxConfig)

		for i := 0; i < layers; i++ {
			db.Snapshot()

			key := common.BigToHash(big.NewInt(int64(i)))
			value := common.BigToHash(big.NewInt(int64(i + 1)))
			db.SetState(address, key, value)
		}
		b.StartTimer()

		db.RevertToSnapshot(0)
	}
}

func BenchmarkRevertToSnapshot1(b *testing.B) {
	benchmarkRevertToSnapshot(b, 1)
}

func BenchmarkRevertToSnapshot4(b *testing.B) {
	benchmarkRevertToSnapshot(b, 4)
}

func BenchmarkRevertToSnapshot8(b *testing.B) {
	benchmarkRevertToSnapshot(b, 8)
}

func BenchmarkRevertToSnapshot16(b *testing.B) {
	benchmarkRevertToSnapshot(b, 16)
}
