package hybridstatedb_test

import (
	"math/big"
	"testing"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/stretchr/testify/suite"

	"github.com/ethereum/go-ethereum/common"
	"github.com/evmos/ethermint/x/evm/hybridstatedb"
	"github.com/evmos/ethermint/x/evm/statedb"
	"github.com/evmos/ethermint/x/evm/testutil"
	"github.com/evmos/ethermint/x/evm/types"
)

var (
	address  common.Address = common.BigToAddress(big.NewInt(101))
	address2 common.Address = common.BigToAddress(big.NewInt(102))
	address3 common.Address = common.BigToAddress(big.NewInt(103))

	blockHash     common.Hash      = common.BigToHash(big.NewInt(9999))
	emptyTxConfig statedb.TxConfig = statedb.NewEmptyTxConfig(blockHash)
)

type HybridStateDBTestSuite struct {
	testutil.TestSuite
}

func TestStateDBTestSuite(t *testing.T) {
	suite.Run(t, &HybridStateDBTestSuite{})
}

func (suite *HybridStateDBTestSuite) TestGetBalance_External() {
	// GetBalance() should account for balance changes from sdk.Context and
	// external factors - e.g. precompiles that modify user account balances.
	keeper := suite.App.EvmKeeper
	params := keeper.GetParams(suite.Ctx)

	tests := []struct {
		name     string
		maleate  func(db *hybridstatedb.StateDB)
		expected *big.Int
	}{
		{
			"no external balance changes",
			func(db *hybridstatedb.StateDB) {
				amount := big.NewInt(100)
				db.AddBalance(address, amount)
			},
			big.NewInt(100),
		},
		{
			"external balance add",
			func(db *hybridstatedb.StateDB) {
				amount := big.NewInt(100)
				db.AddBalance(address, amount)

				// Add some external balance changes, this could be a call to a precompile
				// that transfers funds.
				externalTransferAmount := big.NewInt(50)
				suite.MintCoinsForAccount(
					db.Context(),
					sdk.AccAddress(address.Bytes()),
					sdk.NewCoins(sdk.NewCoin(params.EvmDenom, sdk.NewIntFromBigInt(externalTransferAmount))),
				)
			},
			big.NewInt(150),
		},
	}

	for _, tt := range tests {
		suite.Run(tt.name, func() {
			suite.SetupTest()
			keeper := suite.App.EvmKeeper

			db := hybridstatedb.New(suite.Ctx, keeper, emptyTxConfig)
			tt.maleate(db)

			suite.Require().NoError(db.Commit())

			// New db after Commit()
			db = hybridstatedb.New(suite.Ctx, keeper, emptyTxConfig)

			suite.Require().Equal(
				tt.expected,
				db.GetBalance(address),
				"GetBalance should account for both internal and external balance changes",
			)
		})
	}
}

func (suite *HybridStateDBTestSuite) TestExist() {
	// When precompiles create accounts, it should be visible to StateDB.
	db := hybridstatedb.New(suite.Ctx, suite.App.EvmKeeper, emptyTxConfig)

	suite.Require().False(db.Exist(address), "account shouldn't be created or found")

	// Create account externally
	suite.MintCoinsForAccount(
		db.Context(), // Use the statedb context!
		sdk.AccAddress(address.Bytes()),
		sdk.NewCoins(sdk.NewCoin(types.DefaultEVMDenom, sdk.NewInt(100))),
	)

	// Account should be visible to StateDB - ensures the correct ctx is used
	// in the StateDB
	suite.Require().True(db.Exist(address), "account should be created and found")
}

func (suite *HybridStateDBTestSuite) TestForEachStorage_Committed() {
	db := hybridstatedb.New(suite.Ctx, suite.App.EvmKeeper, emptyTxConfig)
	// Set some storage
	db.SetState(address, common.BytesToHash([]byte("key")), common.BytesToHash([]byte("value")))

	// Commit changes
	suite.Require().NoError(db.Commit())

	// Create a new StateDB
	db = hybridstatedb.New(suite.Ctx, suite.App.EvmKeeper, emptyTxConfig)

	// Iterate over storage
	var keys []common.Hash
	db.ForEachStorage(address, func(key, value common.Hash) bool {
		keys = append(keys, key)
		return false
	})

	suite.Require().Len(keys, 1, "expected 1 key")
	suite.Require().Equal(common.BytesToHash([]byte("key")), keys[0], "expected key to be found")
}

func (suite *HybridStateDBTestSuite) TestForEachStorage_Dirty() {
	db := hybridstatedb.New(suite.Ctx, suite.App.EvmKeeper, emptyTxConfig)
	// Set some storage
	db.SetState(address, common.BytesToHash([]byte("key")), common.BytesToHash([]byte("value")))

	// Iterate over storage
	var keys []common.Hash
	db.ForEachStorage(address, func(key, value common.Hash) bool {
		keys = append(keys, key)
		return false
	})

	suite.Require().Len(keys, 1, "expected 1 key")
	suite.Require().Equal(common.BytesToHash([]byte("key")), keys[0], "expected key to be found")
}
