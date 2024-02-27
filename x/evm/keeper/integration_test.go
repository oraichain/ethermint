package keeper_test

import (
	_ "embed"
	"math/big"
	"testing"

	sdk "github.com/cosmos/cosmos-sdk/types"
	gethparams "github.com/ethereum/go-ethereum/params"

	"github.com/ethereum/go-ethereum/common"
	"github.com/evmos/ethermint/types"
	"github.com/evmos/ethermint/x/evm/testutil"
	"github.com/stretchr/testify/suite"
)

type IntegrationTestSuite struct {
	testutil.TestSuite
}

var test *IntegrationTestSuite

func TestIntegrationTestSuite(t *testing.T) {
	test = new(IntegrationTestSuite)
	test.EnableFeemarket = false
	test.EnableLondonHF = false
	suite.Run(t, test)
}

func (suite *IntegrationTestSuite) TestState_EmptyToEmpty() {
	addr := suite.DeployContract(testutil.StateTestContract)
	storageBefore := suite.GetAllAccountStorage(suite.Ctx, addr)

	contractAcc := suite.App.EvmKeeper.GetAccount(suite.Ctx, addr)
	suite.Require().NotNil(contractAcc)
	suite.Require().Equal(uint64(1), contractAcc.Nonce, "EIP-161: CREATE should increment nonce by 1 over default value (0)")

	_, rsp, err := suite.CallContract(
		testutil.StateTestContract,
		addr,
		common.Big0,
		"tempChangeEmpty",
		big.NewInt(1000),
	)
	suite.Require().NoError(err)
	suite.Require().Emptyf(rsp.VmError, "contract call should not fail: %s", rsp.VmError)

	storageAfter := suite.GetAllAccountStorage(suite.Ctx, addr)
	suite.Equal(storageBefore, storageAfter)
	// 1 key because of the non-empty test, empty test should always be empty
	// even when the contract tries to write 0x0 empty hash.
	suite.Len(storageAfter, 1, "storage should have one key")
}

func (suite *IntegrationTestSuite) TestState_NonEmptyToNonEmpty() {
	addr := suite.DeployContract(testutil.StateTestContract)
	storageBefore := suite.GetAllAccountStorage(suite.Ctx, addr)

	newValue := big.NewInt(1000)
	valueKey := "0x0000000000000000000000000000000000000000000000000000000000000001"

	_, rsp, err := suite.CallContract(
		testutil.StateTestContract,
		addr,
		common.Big0,
		"tempChangeNonEmpty",
		newValue,
	)
	suite.Require().NoError(err)
	suite.Require().Emptyf(rsp.VmError, "contract call should not fail: %s", rsp.VmError)

	storageAfter := suite.GetAllAccountStorage(suite.Ctx, addr)

	// Check that the value at the key has been updated to the new value
	storageBefore[common.HexToHash(valueKey)] = common.BytesToHash(newValue.Bytes())

	suite.Equal(storageBefore, storageAfter)
}

// ----------------------------------------------------------------------------
// EIP-161

func (suite *IntegrationTestSuite) TestEIP158_Enabled() {
	// Params.ChainConfig.EIP158Block needs to be set for
	// EIP-161 to be applied in geth EVM.Call()
	params := suite.App.EvmKeeper.GetParams(suite.Ctx)
	suite.Require().LessOrEqual(
		params.ChainConfig.EIP158Block.Int64(),
		suite.Ctx.BlockHeight(),
		"EIP-158 should be enabled in params",
	)
}

func (suite *IntegrationTestSuite) TestEIP161_CreateNonce() {
	addr := suite.DeployContract(testutil.EIP161TestContract)
	contractAcc := suite.App.EvmKeeper.GetAccount(suite.Ctx, addr)
	suite.Require().NotNil(contractAcc)
	suite.Require().Equal(
		uint64(1),
		contractAcc.Nonce,
		"EIP-161: CREATE should increment nonce by 1 over default value (0)",
	)
}

func (suite *IntegrationTestSuite) TestEIP161_CallGas() {
	addr := suite.DeployContract(testutil.EIP161TestContract)

	// Non-existent account
	targetAddr := common.Address{1, 2, 3, 4, 5, 6}
	targetAcc := suite.App.EvmKeeper.GetAccount(suite.Ctx, targetAddr)
	suite.Require().Nil(targetAcc, "target should not exist")

	var gasUsed1 uint64

	suite.Run("Call 0 value - no 25k gas charge", func() {
		_, rsp, err := suite.CallContract(
			testutil.EIP161TestContract,
			addr,
			common.Big0, // no transfer
			"callAccount",
			targetAddr,
		)
		suite.Require().NoError(err)
		suite.Require().Empty(rsp.VmError)

		targetAcc = suite.App.EvmKeeper.GetAccount(suite.Ctx, targetAddr)
		suite.Require().Nil(
			targetAcc,
			"target should still not exist after being called with no value",
		)

		gasUsed1 = rsp.GasUsed
	})

	suite.Run("Call >0 value - 25k gas charge", func() {
		suite.MintCoinsForAccount(
			suite.Ctx,
			sdk.AccAddress(suite.Address.Bytes()),
			sdk.NewCoins(
				sdk.NewCoin(types.AttoPhoton, sdk.NewInt(100000000000)),
			),
		)

		value := big.NewInt(10000)

		_, rsp, err := suite.CallContract(
			testutil.EIP161TestContract,
			addr,
			value, // >0 value transfer
			"callAccount",
			targetAddr,
		)
		suite.Require().NoError(err)
		suite.Require().Empty(rsp.VmError)

		// Check account + bal
		targetAcc = suite.App.EvmKeeper.GetAccount(suite.Ctx, targetAddr)
		suite.Require().NotNil(
			targetAcc,
			"target should exist after call with value",
		)
		suite.Require().Equal(
			value,
			targetAcc.Balance,
			"target should have received the value",
		)

		// Check gas
		suite.Require().Greater(
			rsp.GasUsed,
			gasUsed1,
			"call with value transfer should use more gas than 0 value call",
		)
		suite.Require().GreaterOrEqual(
			rsp.GasUsed,
			gasUsed1+gethparams.CallNewAccountGas,
			"EIP-161: 25k gas charge when transferring >0 value & destination is dead",
		)
	})
}

// Same as TestEIP161_CallGas but with self destruct
func (suite *IntegrationTestSuite) TestEIP161_SuicideGas() {
	suite.MintCoinsForAccount(
		suite.Ctx,
		sdk.AccAddress(suite.Address.Bytes()),
		sdk.NewCoins(
			sdk.NewCoin(types.AttoPhoton, sdk.NewInt(100000000000)),
		),
	)

	addr := suite.DeployContract(testutil.EIP161TestContract)

	// Non-existent account
	targetAddr := common.Address{10}
	targetAcc := suite.App.EvmKeeper.GetAccount(suite.Ctx, targetAddr)
	suite.Require().Nil(targetAcc, "target should not exist")

	value := big.NewInt(10000)

	var gasUsed1 uint64

	suite.Run("suicide - non-existent destination", func() {
		_, rsp, err := suite.CallContract(
			testutil.EIP161TestContract,
			addr,
			value, // >0 value transfer
			"selfDestructTo",
			targetAddr,
		)
		suite.Require().NoError(err)
		suite.Require().Empty(rsp.VmError)

		gasUsed1 = rsp.GasUsed
	})

	// Deploy again since contract self destructed
	addr = suite.DeployContract(testutil.EIP161TestContract)

	suite.Run("suicide - existing destination", func() {
		targetAcc := suite.App.EvmKeeper.GetAccount(suite.Ctx, targetAddr)
		suite.Require().NotNil(targetAcc, "target should exist")

		_, rsp, err := suite.CallContract(
			testutil.EIP161TestContract,
			addr,
			value, // >0 value transfer
			"selfDestructTo",
			targetAddr, // same target
		)
		suite.Require().NoError(err)
		suite.Require().Empty(rsp.VmError)

		// Check destination balances
		targetAccAfter := suite.App.EvmKeeper.GetAccount(suite.Ctx, targetAddr)
		suite.Require().Equal(
			new(big.Int).Add(targetAcc.Balance, value),
			targetAccAfter.Balance,
			"balance should be transferred",
		)

		// Check gas
		suite.Require().Greater(
			gasUsed1,
			rsp.GasUsed,
			"first call should use more gas - created account",
		)
		suite.Require().GreaterOrEqual(
			gasUsed1,
			rsp.GasUsed+gethparams.CallNewAccountGas,
			"EIP-161: additional 25k gas charge for first call",
		)
	})
}
