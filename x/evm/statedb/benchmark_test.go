package statedb_test

import (
	"math/big"
	"testing"

	"github.com/cosmos/cosmos-sdk/store"
	storetypes "github.com/cosmos/cosmos-sdk/store/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/ethereum/go-ethereum/common"
	"github.com/evmos/ethermint/x/evm/keeper"
	"github.com/evmos/ethermint/x/evm/statedb"
	"github.com/evmos/ethermint/x/evm/testutil"
	"github.com/stretchr/testify/require"
	"github.com/tendermint/tendermint/libs/log"
	tmproto "github.com/tendermint/tendermint/proto/tendermint/types"
	dbm "github.com/tendermint/tm-db"

	ethtypes "github.com/ethereum/go-ethereum/core/types"
)

var (
	testKvStoreKey = sdk.NewKVStoreKey("test")
)

func NewTestContext() sdk.Context {
	db := dbm.NewMemDB()
	cms := store.NewCommitMultiStore(db)

	cms.MountStoreWithDB(testKvStoreKey, storetypes.StoreTypeIAVL, db)
	if err := cms.LoadLatestVersion(); err != nil {
		panic(err)
	}

	return sdk.NewContext(cms, tmproto.Header{}, false, log.NewNopLogger())
}

func benchmarkNestedSnapshot(b *testing.B, layers int) {
	suite := GetTestSuite(b)

	for i := 0; i < b.N; i++ {
		b.StopTimer()
		db := statedb.New(suite.Ctx, suite.App.EvmKeeper, emptyTxConfig)

		// Create layers of nested snapshots
		for i := 0; i < layers; i++ {
			db.Snapshot()

			// Some state change each snapshot
			key := common.BigToHash(big.NewInt(int64(i + 1)))
			value := common.BigToHash(big.NewInt(int64(i + 1)))
			db.SetState(address, key, value)
		}

		b.StartTimer()

		require.NoError(b, db.Commit())
	}
}

func BenchmarkNestedSnapshot1(b *testing.B) {
	benchmarkNestedSnapshot(b, 1)
}

func BenchmarkNestedSnapshot4(b *testing.B) {
	benchmarkNestedSnapshot(b, 4)
}

func BenchmarkNestedSnapshot10(b *testing.B) {
	benchmarkNestedSnapshot(b, 10)
}

func BenchmarkNestedSnapshot100(b *testing.B) {
	benchmarkNestedSnapshot(b, 100)
}

func BenchmarkNestedSnapshot1000(b *testing.B) {
	benchmarkNestedSnapshot(b, 1000)
}

func BenchmarkNestedSnapshot10000(b *testing.B) {
	benchmarkNestedSnapshot(b, 10000)
}

func BenchmarkAddBalance(b *testing.B) {
	suite := GetTestSuite(b)

	db := statedb.New(suite.Ctx, suite.App.EvmKeeper, emptyTxConfig)

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		db.AddBalance(address, big.NewInt(1))
	}
}

func BenchmarkSubBalance(b *testing.B) {
	suite := GetTestSuite(b)
	db := statedb.New(suite.Ctx, suite.App.EvmKeeper, emptyTxConfig)

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		db.SubBalance(address, big.NewInt(1))
	}
}

func BenchmarkGetBalance(b *testing.B) {
	suite := GetTestSuite(b)
	db := statedb.New(suite.Ctx, suite.App.EvmKeeper, emptyTxConfig)

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		db.GetBalance(address)
	}
}

func BenchmarkGetNonce(b *testing.B) {
	suite := GetTestSuite(b)
	db := statedb.New(suite.Ctx, suite.App.EvmKeeper, emptyTxConfig)

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		db.GetNonce(address)
	}
}

func BenchmarkSetNonce(b *testing.B) {
	suite := GetTestSuite(b)
	db := statedb.New(suite.Ctx, suite.App.EvmKeeper, emptyTxConfig)

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		db.SetNonce(address, 1)
	}
}

func BenchmarkGetCodeHash(b *testing.B) {
	suite := GetTestSuite(b)
	db := statedb.New(suite.Ctx, suite.App.EvmKeeper, emptyTxConfig)

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		db.GetCodeHash(address)
	}
}

func BenchmarkGetCode(b *testing.B) {
	suite := GetTestSuite(b)
	db := statedb.New(suite.Ctx, suite.App.EvmKeeper, emptyTxConfig)

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		db.GetCode(address)
	}
}

func BenchmarkAddLog(b *testing.B) {
	suite := GetTestSuite(b)
	db := statedb.New(suite.Ctx, suite.App.EvmKeeper, emptyTxConfig)
	log := &ethtypes.Log{
		Address: address,
		Topics:  []common.Hash{common.BigToHash(big.NewInt(1))},
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		db.AddLog(log)
	}
}

func benchmarkGetLogs(b *testing.B, logEntries int) {
	suite := GetTestSuite(b)
	db := statedb.New(suite.Ctx, suite.App.EvmKeeper, emptyTxConfig)

	for i := 0; i < logEntries; i++ {
		log := &ethtypes.Log{
			Address: address,
			Topics:  []common.Hash{common.BigToHash(big.NewInt(int64(i)))},
		}
		db.AddLog(log)
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		db.Logs()
	}
}

func BenchmarkGetLogs1(b *testing.B) {
	benchmarkGetLogs(b, 1)
}

func BenchmarkGetLogs8(b *testing.B) {
	benchmarkGetLogs(b, 8)
}

func BenchmarkGetLogs64(b *testing.B) {
	benchmarkGetLogs(b, 64)
}

func BenchmarkGetLogs512(b *testing.B) {
	benchmarkGetLogs(b, 512)
}

func GetTestSuite(b *testing.B) *testutil.KeeperTestSuite {
	// Just reuse the keeper test suite to setup and create a testing app
	suite := testutil.KeeperTestSuite{}
	suite.SetupTestWithT(b)

	return &suite
}

func GetTestKeeper() *keeper.Keeper {
	// Just reuse the keeper test suite to setup and create a keeper
	suite := testutil.KeeperTestSuite{}
	suite.SetupTest()

	return suite.App.EvmKeeper
}
