package keeper_test

import (
	_ "embed"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
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
	test.EnableLondonHF = true
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
