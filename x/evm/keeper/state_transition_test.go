package keeper_test

import (
	"bytes"
	"fmt"
	"math"
	"math/big"
	"math/rand"
	"sort"
	"strings"

	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	"github.com/cosmos/cosmos-sdk/store/iavl"
	storetypes "github.com/cosmos/cosmos-sdk/store/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/core/vm"
	"github.com/ethereum/go-ethereum/params"
	"github.com/evmos/ethermint/tests"
	"github.com/evmos/ethermint/x/evm/keeper"
	"github.com/evmos/ethermint/x/evm/statedb"
	"github.com/evmos/ethermint/x/evm/statedb_legacy"
	"github.com/evmos/ethermint/x/evm/types"
	"github.com/tendermint/tendermint/crypto/tmhash"
	tmproto "github.com/tendermint/tendermint/proto/tendermint/types"
	tmtypes "github.com/tendermint/tendermint/types"

	cosmosiavl "github.com/cosmos/iavl"
)

func (suite *KeeperTestSuite) TestGetHashFn() {
	header := suite.Ctx.BlockHeader()
	h, _ := tmtypes.HeaderFromProto(&header)
	hash := h.Hash()

	testCases := []struct {
		msg      string
		height   uint64
		malleate func()
		expHash  common.Hash
	}{
		{
			"case 1.1: context hash cached",
			uint64(suite.Ctx.BlockHeight()),
			func() {
				suite.Ctx = suite.Ctx.WithHeaderHash(tmhash.Sum([]byte("header")))
			},
			common.BytesToHash(tmhash.Sum([]byte("header"))),
		},
		{
			"case 1.2: failed to cast Tendermint header",
			uint64(suite.Ctx.BlockHeight()),
			func() {
				header := tmproto.Header{}
				header.Height = suite.Ctx.BlockHeight()
				suite.Ctx = suite.Ctx.WithBlockHeader(header)
			},
			common.Hash{},
		},
		{
			"case 1.3: hash calculated from Tendermint header",
			uint64(suite.Ctx.BlockHeight()),
			func() {
				suite.Ctx = suite.Ctx.WithBlockHeader(header)
			},
			common.BytesToHash(hash),
		},
		{
			"case 2.1: height lower than current one, hist info not found",
			1,
			func() {
				suite.Ctx = suite.Ctx.WithBlockHeight(10)
			},
			common.Hash{},
		},
		{
			"case 2.2: height lower than current one, invalid hist info header",
			1,
			func() {
				suite.App.StakingKeeper.SetHistoricalInfo(suite.Ctx, 1, &stakingtypes.HistoricalInfo{})
				suite.Ctx = suite.Ctx.WithBlockHeight(10)
			},
			common.Hash{},
		},
		{
			"case 2.3: height lower than current one, calculated from hist info header",
			1,
			func() {
				histInfo := &stakingtypes.HistoricalInfo{
					Header: header,
				}
				suite.App.StakingKeeper.SetHistoricalInfo(suite.Ctx, 1, histInfo)
				suite.Ctx = suite.Ctx.WithBlockHeight(10)
			},
			common.BytesToHash(hash),
		},
		{
			"case 3: height greater than current one",
			200,
			func() {},
			common.Hash{},
		},
	}

	for _, tc := range testCases {
		suite.Run(fmt.Sprintf("Case %s", tc.msg), func() {
			suite.SetupTest() // reset

			tc.malleate()

			hash := suite.App.EvmKeeper.GetHashFn(suite.Ctx)(tc.height)
			suite.Require().Equal(tc.expHash, hash)
		})
	}
}

func (suite *KeeperTestSuite) TestGetCoinbaseAddress() {
	valOpAddr := tests.GenerateAddress()

	testCases := []struct {
		msg      string
		malleate func()
		expPass  bool
	}{
		{
			"validator not found",
			func() {
				header := suite.Ctx.BlockHeader()
				header.ProposerAddress = []byte{}
				suite.Ctx = suite.Ctx.WithBlockHeader(header)
			},
			false,
		},
		{
			"success",
			func() {
				valConsAddr, privkey := tests.NewAddrKey()

				pkAny, err := codectypes.NewAnyWithValue(privkey.PubKey())
				suite.Require().NoError(err)

				validator := stakingtypes.Validator{
					OperatorAddress: sdk.ValAddress(valOpAddr.Bytes()).String(),
					ConsensusPubkey: pkAny,
				}

				suite.App.StakingKeeper.SetValidator(suite.Ctx, validator)
				err = suite.App.StakingKeeper.SetValidatorByConsAddr(suite.Ctx, validator)
				suite.Require().NoError(err)

				header := suite.Ctx.BlockHeader()
				header.ProposerAddress = valConsAddr.Bytes()
				suite.Ctx = suite.Ctx.WithBlockHeader(header)

				_, found := suite.App.StakingKeeper.GetValidatorByConsAddr(suite.Ctx, valConsAddr.Bytes())
				suite.Require().True(found)

				suite.Require().NotEmpty(suite.Ctx.BlockHeader().ProposerAddress)
			},
			true,
		},
	}

	for _, tc := range testCases {
		suite.Run(fmt.Sprintf("Case %s", tc.msg), func() {
			suite.SetupTest() // reset

			tc.malleate()
			proposerAddress := suite.Ctx.BlockHeader().ProposerAddress
			coinbase, err := suite.App.EvmKeeper.GetCoinbaseAddress(suite.Ctx, sdk.ConsAddress(proposerAddress))
			if tc.expPass {
				suite.Require().NoError(err)
				suite.Require().Equal(valOpAddr, coinbase)
			} else {
				suite.Require().Error(err)
			}
		})
	}
}

func (suite *KeeperTestSuite) TestGetEthIntrinsicGas() {
	testCases := []struct {
		name               string
		data               []byte
		accessList         ethtypes.AccessList
		height             int64
		isContractCreation bool
		noError            bool
		expGas             uint64
	}{
		{
			"no data, no accesslist, not contract creation, not homestead, not istanbul",
			nil,
			nil,
			1,
			false,
			true,
			params.TxGas,
		},
		{
			"with one zero data, no accesslist, not contract creation, not homestead, not istanbul",
			[]byte{0},
			nil,
			1,
			false,
			true,
			params.TxGas + params.TxDataZeroGas*1,
		},
		{
			"with one non zero data, no accesslist, not contract creation, not homestead, not istanbul",
			[]byte{1},
			nil,
			1,
			true,
			true,
			params.TxGas + params.TxDataNonZeroGasFrontier*1,
		},
		{
			"no data, one accesslist, not contract creation, not homestead, not istanbul",
			nil,
			[]ethtypes.AccessTuple{
				{},
			},
			1,
			false,
			true,
			params.TxGas + params.TxAccessListAddressGas,
		},
		{
			"no data, one accesslist with one storageKey, not contract creation, not homestead, not istanbul",
			nil,
			[]ethtypes.AccessTuple{
				{StorageKeys: make([]common.Hash, 1)},
			},
			1,
			false,
			true,
			params.TxGas + params.TxAccessListAddressGas + params.TxAccessListStorageKeyGas*1,
		},
		{
			"no data, no accesslist, is contract creation, is homestead, not istanbul",
			nil,
			nil,
			2,
			true,
			true,
			params.TxGasContractCreation,
		},
		{
			"with one zero data, no accesslist, not contract creation, is homestead, is istanbul",
			[]byte{1},
			nil,
			3,
			false,
			true,
			params.TxGas + params.TxDataNonZeroGasEIP2028*1,
		},
	}

	for _, tc := range testCases {
		suite.Run(fmt.Sprintf("Case %s", tc.name), func() {
			suite.SetupTest() // reset

			params := suite.App.EvmKeeper.GetParams(suite.Ctx)
			ethCfg := params.ChainConfig.EthereumConfig(suite.App.EvmKeeper.ChainID())
			ethCfg.HomesteadBlock = big.NewInt(2)
			ethCfg.IstanbulBlock = big.NewInt(3)
			signer := ethtypes.LatestSignerForChainID(suite.App.EvmKeeper.ChainID())

			suite.Ctx = suite.Ctx.WithBlockHeight(tc.height)

			nonce := suite.App.EvmKeeper.GetNonce(suite.Ctx, suite.Address)
			m, err := newNativeMessage(
				nonce,
				suite.Ctx.BlockHeight(),
				suite.Address,
				ethCfg,
				suite.Signer,
				signer,
				ethtypes.AccessListTxType,
				tc.data,
				tc.accessList,
			)
			suite.Require().NoError(err)

			gas, err := suite.App.EvmKeeper.GetEthIntrinsicGas(suite.Ctx, m, ethCfg, tc.isContractCreation)
			if tc.noError {
				suite.Require().NoError(err)
			} else {
				suite.Require().Error(err)
			}

			suite.Require().Equal(tc.expGas, gas)
		})
	}
}

func (suite *KeeperTestSuite) TestGasToRefund() {
	testCases := []struct {
		name           string
		gasconsumed    uint64
		refundQuotient uint64
		expGasRefund   uint64
		expPanic       bool
	}{
		{
			"gas refund 5",
			5,
			1,
			5,
			false,
		},
		{
			"gas refund 10",
			10,
			1,
			10,
			false,
		},
		{
			"gas refund availableRefund",
			11,
			1,
			10,
			false,
		},
		{
			"gas refund quotient 0",
			11,
			0,
			0,
			true,
		},
	}

	for _, tc := range testCases {
		suite.Run(fmt.Sprintf("Case %s", tc.name), func() {
			suite.MintFeeCollector = true
			suite.SetupTest() // reset
			vmdb := suite.StateDB()
			vmdb.AddRefund(10)

			if tc.expPanic {
				panicF := func() {
					keeper.GasToRefund(vmdb.GetRefund(), tc.gasconsumed, tc.refundQuotient)
				}
				suite.Require().Panics(panicF)
			} else {
				gr := keeper.GasToRefund(vmdb.GetRefund(), tc.gasconsumed, tc.refundQuotient)
				suite.Require().Equal(tc.expGasRefund, gr)
			}
		})
	}
	suite.MintFeeCollector = false
}

func (suite *KeeperTestSuite) TestRefundGas() {
	var (
		m   core.Message
		err error
	)

	testCases := []struct {
		name           string
		leftoverGas    uint64
		refundQuotient uint64
		noError        bool
		expGasRefund   uint64
		malleate       func()
	}{
		{
			name:           "leftoverGas more than tx gas limit",
			leftoverGas:    params.TxGas + 1,
			refundQuotient: params.RefundQuotient,
			noError:        false,
			expGasRefund:   params.TxGas + 1,
		},
		{
			name:           "leftoverGas equal to tx gas limit, insufficient fee collector account",
			leftoverGas:    params.TxGas,
			refundQuotient: params.RefundQuotient,
			noError:        true,
			expGasRefund:   0,
		},
		{
			name:           "leftoverGas less than to tx gas limit",
			leftoverGas:    params.TxGas - 1,
			refundQuotient: params.RefundQuotient,
			noError:        true,
			expGasRefund:   0,
		},
		{
			name:           "no leftoverGas, refund half used gas ",
			leftoverGas:    0,
			refundQuotient: params.RefundQuotient,
			noError:        true,
			expGasRefund:   params.TxGas / params.RefundQuotient,
		},
		{
			name:           "invalid Gas value in msg",
			leftoverGas:    0,
			refundQuotient: params.RefundQuotient,
			noError:        false,
			expGasRefund:   params.TxGas,
			malleate: func() {
				keeperParams := suite.App.EvmKeeper.GetParams(suite.Ctx)
				m, err = suite.createContractGethMsg(
					suite.StateDB().GetNonce(suite.Address),
					ethtypes.LatestSignerForChainID(suite.App.EvmKeeper.ChainID()),
					keeperParams.ChainConfig.EthereumConfig(suite.App.EvmKeeper.ChainID()),
					big.NewInt(-100),
				)
				suite.Require().NoError(err)
			},
		},
	}

	for _, tc := range testCases {
		suite.Run(fmt.Sprintf("Case %s", tc.name), func() {
			suite.MintFeeCollector = true
			suite.SetupTest() // reset

			keeperParams := suite.App.EvmKeeper.GetParams(suite.Ctx)
			ethCfg := keeperParams.ChainConfig.EthereumConfig(suite.App.EvmKeeper.ChainID())
			signer := ethtypes.LatestSignerForChainID(suite.App.EvmKeeper.ChainID())
			vmdb := suite.StateDB()

			m, err = newNativeMessage(
				vmdb.GetNonce(suite.Address),
				suite.Ctx.BlockHeight(),
				suite.Address,
				ethCfg,
				suite.Signer,
				signer,
				ethtypes.AccessListTxType,
				nil,
				nil,
			)
			suite.Require().NoError(err)

			vmdb.AddRefund(params.TxGas)

			if tc.leftoverGas > m.Gas() {
				return
			}

			if tc.malleate != nil {
				tc.malleate()
			}

			gasUsed := m.Gas() - tc.leftoverGas
			refund := keeper.GasToRefund(vmdb.GetRefund(), gasUsed, tc.refundQuotient)
			suite.Require().Equal(tc.expGasRefund, refund)

			err = suite.App.EvmKeeper.RefundGas(suite.Ctx, m, refund, "aphoton")
			if tc.noError {
				suite.Require().NoError(err)
			} else {
				suite.Require().Error(err)
			}
		})
	}
	suite.MintFeeCollector = false
}

func (suite *KeeperTestSuite) TestResetGasMeterAndConsumeGas() {
	testCases := []struct {
		name        string
		gasConsumed uint64
		gasUsed     uint64
		expPanic    bool
	}{
		{
			"gas consumed 5, used 5",
			5,
			5,
			false,
		},
		{
			"gas consumed 5, used 10",
			5,
			10,
			false,
		},
		{
			"gas consumed 10, used 10",
			10,
			10,
			false,
		},
		{
			"gas consumed 11, used 10, NegativeGasConsumed panic",
			11,
			10,
			true,
		},
		{
			"gas consumed 1, used 10, overflow panic",
			1,
			math.MaxUint64,
			true,
		},
	}

	for _, tc := range testCases {
		suite.Run(fmt.Sprintf("Case %s", tc.name), func() {
			suite.SetupTest() // reset

			panicF := func() {
				gm := sdk.NewGasMeter(10)
				gm.ConsumeGas(tc.gasConsumed, "")
				ctx := suite.Ctx.WithGasMeter(gm)
				suite.App.EvmKeeper.ResetGasMeterAndConsumeGas(ctx, tc.gasUsed)
			}

			if tc.expPanic {
				suite.Require().Panics(panicF)
			} else {
				suite.Require().NotPanics(panicF)
			}
		})
	}
}

func (suite *KeeperTestSuite) TestEVMConfig() {
	proposerAddress := suite.Ctx.BlockHeader().ProposerAddress
	cfg, err := suite.App.EvmKeeper.EVMConfig(suite.Ctx, proposerAddress, big.NewInt(9000))
	suite.Require().NoError(err)
	suite.Require().Equal(types.DefaultParams(), cfg.Params)
	// london hardfork is enabled by default
	suite.Require().Equal(big.NewInt(0), cfg.BaseFee)
	suite.Require().Equal(suite.Address, cfg.CoinBase)
	suite.Require().Equal(types.DefaultParams().ChainConfig.EthereumConfig(big.NewInt(9000)), cfg.ChainConfig)
}

func (suite *KeeperTestSuite) TestContractDeployment() {
	contractAddress := suite.DeployTestContract(suite.T(), suite.Address, big.NewInt(10000000000000))
	db := suite.StateDB()
	suite.Require().Greater(db.GetCodeSize(contractAddress), 0)
}

func (suite *KeeperTestSuite) TestApplyMessage() {
	expectedGasUsed := params.TxGas
	var msg core.Message

	proposerAddress := suite.Ctx.BlockHeader().ProposerAddress
	config, err := suite.App.EvmKeeper.EVMConfig(suite.Ctx, proposerAddress, big.NewInt(9000))
	suite.Require().NoError(err)

	keeperParams := suite.App.EvmKeeper.GetParams(suite.Ctx)
	chainCfg := keeperParams.ChainConfig.EthereumConfig(suite.App.EvmKeeper.ChainID())
	signer := ethtypes.LatestSignerForChainID(suite.App.EvmKeeper.ChainID())
	tracer := suite.App.EvmKeeper.Tracer(suite.Ctx, msg, config.ChainConfig)
	vmdb := suite.StateDB()

	msg, err = newNativeMessage(
		vmdb.GetNonce(suite.Address),
		suite.Ctx.BlockHeight(),
		suite.Address,
		chainCfg,
		suite.Signer,
		signer,
		ethtypes.AccessListTxType,
		nil,
		nil,
	)
	suite.Require().NoError(err)

	res, err := suite.App.EvmKeeper.ApplyMessage(suite.Ctx, msg, tracer, true)

	suite.Require().NoError(err)
	suite.Require().Equal(expectedGasUsed, res.GasUsed)
	suite.Require().False(res.Failed())
}

func (suite *KeeperTestSuite) TestApplyMessageWithConfig() {
	var (
		msg             core.Message
		err             error
		expectedGasUsed uint64
		config          *statedb.EVMConfig
		keeperParams    types.Params
		signer          ethtypes.Signer
		vmdb            *statedb.StateDB
		txConfig        statedb.TxConfig
		chainCfg        *params.ChainConfig
	)

	testCases := []struct {
		name     string
		malleate func()
		expErr   bool
	}{
		{
			"messsage applied ok",
			func() {
				msg, err = newNativeMessage(
					vmdb.GetNonce(suite.Address),
					suite.Ctx.BlockHeight(),
					suite.Address,
					chainCfg,
					suite.Signer,
					signer,
					ethtypes.AccessListTxType,
					nil,
					nil,
				)
				suite.Require().NoError(err)
			},
			false,
		},
		{
			"call contract tx with config param EnableCall = false",
			func() {
				config.Params.EnableCall = false
				msg, err = newNativeMessage(
					vmdb.GetNonce(suite.Address),
					suite.Ctx.BlockHeight(),
					suite.Address,
					chainCfg,
					suite.Signer,
					signer,
					ethtypes.AccessListTxType,
					nil,
					nil,
				)
				suite.Require().NoError(err)
			},
			true,
		},
		{
			"create contract tx with config param EnableCreate = false",
			func() {
				msg, err = suite.createContractGethMsg(vmdb.GetNonce(suite.Address), signer, chainCfg, big.NewInt(1))
				suite.Require().NoError(err)
				config.Params.EnableCreate = false
			},
			true,
		},
	}

	for _, tc := range testCases {
		suite.Run(fmt.Sprintf("Case %s", tc.name), func() {
			suite.SetupTest()
			expectedGasUsed = params.TxGas

			proposerAddress := suite.Ctx.BlockHeader().ProposerAddress
			config, err = suite.App.EvmKeeper.EVMConfig(suite.Ctx, proposerAddress, big.NewInt(9000))
			suite.Require().NoError(err)

			keeperParams = suite.App.EvmKeeper.GetParams(suite.Ctx)
			chainCfg = keeperParams.ChainConfig.EthereumConfig(suite.App.EvmKeeper.ChainID())
			signer = ethtypes.LatestSignerForChainID(suite.App.EvmKeeper.ChainID())
			vmdb = suite.StateDB()
			txConfig = suite.App.EvmKeeper.TxConfig(suite.Ctx, common.Hash{})

			tc.malleate()
			res, err := suite.App.EvmKeeper.ApplyMessageWithConfig(suite.Ctx, msg, nil, true, config, txConfig)

			if tc.expErr {
				suite.Require().Error(err)
				return
			}

			suite.Require().NoError(err)
			suite.Require().False(res.Failed())
			suite.Require().Equal(expectedGasUsed, res.GasUsed)
		})
	}
}

func (suite *KeeperTestSuite) createContractGethMsg(nonce uint64, signer ethtypes.Signer, cfg *params.ChainConfig, gasPrice *big.Int) (core.Message, error) {
	ethMsg, err := suite.createContractMsgTx(nonce, signer, cfg, gasPrice)
	if err != nil {
		return nil, err
	}

	msgSigner := ethtypes.MakeSigner(cfg, big.NewInt(suite.Ctx.BlockHeight()))
	return ethMsg.AsMessage(msgSigner, nil)
}

func (suite *KeeperTestSuite) createContractMsgTx(nonce uint64, signer ethtypes.Signer, cfg *params.ChainConfig, gasPrice *big.Int) (*types.MsgEthereumTx, error) {
	contractCreateTx := &ethtypes.AccessListTx{
		GasPrice: gasPrice,
		Gas:      params.TxGasContractCreation * 2,
		To:       nil,
		// Minimal contract data
		// https://ethereum.stackexchange.com/questions/40757/what-is-the-shortest-bytecode-that-will-publish-a-contract-with-non-zero-bytecod
		// Using the previous string "contract_data" as contract code may cause
		// an error as it includes 0x5f which is PUSH0, only on Shanghai and later
		Data:  common.Hex2Bytes("0x3859818153F3"),
		Nonce: nonce,
	}
	ethTx := ethtypes.NewTx(contractCreateTx)
	ethMsg := &types.MsgEthereumTx{}
	ethMsg.FromEthereumTx(ethTx)
	ethMsg.From = suite.Address.Hex()

	return ethMsg, ethMsg.Sign(signer, suite.Signer)
}

func (suite *KeeperTestSuite) TestGetProposerAddress() {
	var a sdk.ConsAddress
	address := sdk.ConsAddress(suite.Address.Bytes())
	proposerAddress := sdk.ConsAddress(suite.Ctx.BlockHeader().ProposerAddress)
	testCases := []struct {
		msg    string
		adr    sdk.ConsAddress
		expAdr sdk.ConsAddress
	}{
		{
			"proposer address provided",
			address,
			address,
		},
		{
			"nil proposer address provided",
			nil,
			proposerAddress,
		},
		{
			"typed nil proposer address provided",
			a,
			proposerAddress,
		},
	}
	for _, tc := range testCases {
		suite.Run(fmt.Sprintf("Case %s", tc.msg), func() {
			suite.Require().Equal(tc.expAdr, keeper.GetProposerAddress(suite.Ctx, tc.adr))
		})
	}
}

var (
	blockHash     common.Hash      = common.BigToHash(big.NewInt(9999))
	emptyTxConfig statedb.TxConfig = statedb.NewEmptyTxConfig(blockHash)
)

type StateDBCommit interface {
	vm.StateDB
	Commit() error
}

func (suite *KeeperTestSuite) TestAccountNumberOrder() {
	// New accounts should have account numbers in sorted order on Commit(),
	// NOT when they were first touched.

	// Relevant journal based StateDB code:
	// https://github.com/Kava-Labs/ethermint/blob/877e8fd1bd140c37ad05ed613f31e28f0130c0c4/x/evm/statedb/statedb.go#L464-L466
	// - New accounts are persisted in journal without an account number until
	//   Commit() is called, which iterates new accounts sorted by address and
	//   assigns account numbers in order.
	// - Every method that calls getOrNewStateObject will have the account
	//   created in Commit() if it doesn't exist yet.

	// This test ensures all the specified StateDB methods that should create
	// accounts do so in a way that results in sorted account numbers in the
	// following cases:
	// - When accounts are touched in ascending order (by address)
	// - When accounts are touched in descending order (by address)
	// - When accounts are touched in random order

	type MethodTest struct {
		name  string
		touch func(vmdb vm.StateDB, addr common.Address)
	}

	tests := []MethodTest{
		{
			"SetState",
			func(vmdb vm.StateDB, addr common.Address) {
				key := common.BytesToHash([]byte{1})
				value := common.BytesToHash([]byte{1})
				vmdb.SetState(addr, key, value)
			},
		},
		{
			"SetCode",
			func(vmdb vm.StateDB, addr common.Address) {
				vmdb.SetCode(addr, []byte{1})
			},
		},
		{
			"SetNonce",
			func(vmdb vm.StateDB, addr common.Address) {
				vmdb.SetNonce(addr, 2)
			},
		},
		{
			"AddBalance",
			func(vmdb vm.StateDB, addr common.Address) {
				vmdb.AddBalance(addr, big.NewInt(5))
			},
		},
		{
			"SubBalance",
			func(vmdb vm.StateDB, addr common.Address) {
				vmdb.SubBalance(addr, big.NewInt(0))
			},
		},
	}

	// -------------------------------------------------------------------------
	// Addresses setup -> All of these should have the same account numbers
	// - Ascending order
	// - Descending order
	// - Random order

	orderedAddrs := []common.Address{}
	for i := 0; i < 5; i++ {
		num := big.NewInt(int64(i + 1))
		addr := common.BigToAddress(num)
		orderedAddrs = append(orderedAddrs, addr)
	}

	reversedAddrs := make([]common.Address, len(orderedAddrs))
	copy(reversedAddrs, orderedAddrs)

	for i := len(reversedAddrs)/2 - 1; i >= 0; i-- {
		opp := len(reversedAddrs) - 1 - i
		reversedAddrs[i], reversedAddrs[opp] = reversedAddrs[opp], reversedAddrs[i]
	}

	// Make sure it's actually reversed now, descending order
	for i := 0; i < len(reversedAddrs)-1; i++ {
		suite.Require().True(
			bytes.Compare(reversedAddrs[i].Bytes(), reversedAddrs[i+1].Bytes()) > 0,
			"addresses should be in descending order",
		)
	}

	randomAddrs := make([]common.Address, len(orderedAddrs))
	copy(randomAddrs, orderedAddrs)

	// Shuffle
	addrsShuffled := make([]common.Address, len(randomAddrs))
	perm := rand.Perm(len(randomAddrs))
	for i, v := range perm {
		addrsShuffled[v] = randomAddrs[i]
	}

	// -------------------------------------------------------------------------
	// Shared test logic

	testFn := func(
		stateDBConstructor func() StateDBCommit,
		addrs []common.Address,
		tt MethodTest,
	) {
		// Reset app state
		suite.SetupTest()

		// Ensure evm account already exists - otherwise it will be created on
		// the first balance change in an account and have an account number gap
		// in the user accounts (SetAccount -> SetBalance -> MintCoins)
		_ = suite.App.AccountKeeper.GetModuleAccount(suite.Ctx, types.ModuleName)

		for _, addr := range addrs {
			acc := suite.App.AccountKeeper.GetAccount(suite.Ctx, addr.Bytes())
			suite.Require().Nil(acc, "account should not exist yet")
		}

		db := stateDBConstructor()

		for _, addr := range addrs {
			// Run the corresponding StateDB method that should touch an account
			tt.touch(db, addr)
		}

		suite.Require().NoError(db.Commit())
		suite.Commit()

		// Check account numbers
		accounts := make([]authtypes.AccountI, len(addrs))
		for i, addr := range addrs {
			acc := suite.App.AccountKeeper.GetAccount(suite.Ctx, addr.Bytes())
			suite.Require().NotNil(acc, "account should exist")
			accounts[i] = acc
		}

		sort.Slice(accounts, func(i, j int) bool {
			return bytes.Compare(accounts[i].GetAddress().Bytes(), accounts[j].GetAddress().Bytes()) < 0
		})

		// Ensure account numbers are in order
		for i := 0; i < len(accounts)-1; i++ {
			accNum1 := accounts[i].GetAccountNumber()
			accNum2 := accounts[i+1].GetAccountNumber()

			suite.Require().Less(
				accNum1,
				accNum2,
				"account numbers should be ascending order",
			)
			suite.Require().Equalf(
				accNum1+1,
				accNum2,
				"account numbers should be in order, %v",
				accounts,
			)
		}
	}

	ctxStateDBConstructor := func() StateDBCommit {
		return statedb.New(suite.Ctx, suite.App.EvmKeeper, emptyTxConfig)
	}
	legacyStateDBConstructor := func() StateDBCommit {
		return statedb_legacy.New(suite.Ctx, suite.App.EvmKeeper, emptyTxConfig)
	}

	// Run the tests
	for _, tt := range tests {
		// First run the tests against legacy statedb to ensure the correct
		// behavior is expected
		suite.Run(tt.name+"_legacy", func() {
			testFn(legacyStateDBConstructor, orderedAddrs, tt)
		})

		suite.Run(tt.name+"_reversed_legacy", func() {
			testFn(legacyStateDBConstructor, reversedAddrs, tt)
		})

		suite.Run(tt.name+"_random_legacy", func() {
			testFn(legacyStateDBConstructor, addrsShuffled, tt)
		})
	}

	return

	// CacheCtx statedb
	for _, tt := range tests {
		suite.Run(tt.name, func() {
			testFn(ctxStateDBConstructor, orderedAddrs, tt)
		})

		// Now do the same but reversed addresses
		suite.Run(tt.name+"_reversed", func() {
			testFn(ctxStateDBConstructor, reversedAddrs, tt)
		})

		// And again! but with random order
		suite.Run(tt.name+"_random", func() {
			testFn(ctxStateDBConstructor, addrsShuffled, tt)
		})
	}
}

func (suite *KeeperTestSuite) TestNoopStateChange_UnmodifiedIAVLTree() {
	// suite.T().Skip("CacheCtx StateDB does not currently skip noop state changes")

	// On StateDB.Commit(), if there is a dirty state change that matches the
	// committed state, it should be skipped. Only state changes that are
	// different from committed state should be applied to the underlying store.
	// Corresponding journal based StateDB code:
	// https://github.com/ethereum/go-ethereum/blob/e31709db6570e302557a9bccd681034ea0dcc246/core/state/state_object.go#L302-L305
	// https://github.com/Kava-Labs/ethermint/blob/877e8fd1bd140c37ad05ed613f31e28f0130c0c4/x/evm/statedb/statedb.go#L469-L472

	// Even with store.Set() on the same pre-existing key and value, it will
	// update the underlying iavl.Node version and thus the node hash, parent
	// hashes, and the commitID

	// E.g. SetState(A, B) -> xxx -> SetState(A, B) should not be applied to
	// the underlying store.
	// xxx could be 0 or more state changes to the same key. It can be different
	// values since the only value that actually matters is the last one when
	// Commit() is called.

	addr := common.BigToAddress(big.NewInt(1))
	key := common.BigToHash(big.NewInt(10))
	value := common.BigToHash(big.NewInt(20))

	tests := []struct {
		name            string
		initializeState func(vmdb vm.StateDB)
		maleate         func(vmdb vm.StateDB)
	}{
		{
			"SetState - no extra snapshots",
			func(vmdb vm.StateDB) {
				vmdb.SetState(addr, key, value)
			},
			func(vmdb vm.StateDB) {
				vmdb.SetState(addr, key, value)
			},
		},
		{
			"SetState - 2nd snapshot, same value",
			func(vmdb vm.StateDB) {
				vmdb.SetState(addr, key, value)
			},
			func(vmdb vm.StateDB) {
				// A -> A
				vmdb.SetState(addr, key, value)

				// Same value, just different snapshot. Should be skipped in all
				// snapshots.
				_ = vmdb.Snapshot()
				vmdb.SetState(addr, key, value)
			},
		},
		{
			"SetState - 2nd snapshot, different value",
			func(vmdb vm.StateDB) {
				vmdb.SetState(addr, key, value)
			},
			func(vmdb vm.StateDB) {
				// A -> B -> A

				// Different value in 1st snapshot
				value2 := common.BigToHash(big.NewInt(30))
				vmdb.SetState(addr, key, value2)

				// Back to original value in 2nd snapshot
				_ = vmdb.Snapshot()
				vmdb.SetState(addr, key, value)
			},
		},
		{
			"SetState - multiple snapshots, different value",
			func(vmdb vm.StateDB) {
				vmdb.SetState(addr, key, value)
			},
			func(vmdb vm.StateDB) {
				// A -> B -> C -> A

				// Different value in 1st snapshot
				value2 := common.BigToHash(big.NewInt(30))
				value3 := common.BigToHash(big.NewInt(40))
				_ = vmdb.Snapshot()
				vmdb.SetState(addr, key, value2)

				// Extra empty snapshot for good measure
				_ = vmdb.Snapshot()

				_ = vmdb.Snapshot()
				vmdb.SetState(addr, key, value3)

				// Back to original value in last snapshot
				_ = vmdb.Snapshot()
				vmdb.SetState(addr, key, value)
			},
		},
	}

	for _, tt := range tests {
		suite.Run(tt.name, func() {
			// reset
			suite.SetupTest()

			db := statedb.New(suite.Ctx, suite.App.EvmKeeper, emptyTxConfig)
			tt.initializeState(db)

			suite.Require().NoError(db.Commit())
			suite.Commit()

			store := suite.App.CommitMultiStore().GetStore(suite.App.GetKey(types.StoreKey))
			iavlStore := store.(*iavl.Store)
			commitID1 := iavlStore.LastCommitID()

			// New statedb that should not modify the underlying store
			db = statedb.New(suite.Ctx, suite.App.EvmKeeper, emptyTxConfig)
			tt.maleate(db)

			suite.Require().NoError(db.Commit())
			suite.Commit()

			commitID2 := iavlStore.LastCommitID()

			// We can compare the commitIDs since this is *only* the x/evm store which
			// doesn't change between blocks without state changes. Any version change,
			// e.g. no-op change that was written when it shouldn't, will modify the
			// hash.
			suite.Require().Equal(
				common.Bytes2Hex(commitID1.Hash),
				common.Bytes2Hex(commitID2.Hash),
				"evm store should be unchanged",
			)

		})
	}
}

func (suite *KeeperTestSuite) TestStateDB_IAVLConsistency() {
	// evm store keys prefixes:
	// Code = 1
	// Storage = 2
	addr1 := common.BigToAddress(big.NewInt(1))
	addr2 := common.BigToAddress(big.NewInt(2))

	tests := []struct {
		name       string
		maleate    func(vmdb vm.StateDB)
		shouldSkip bool
	}{
		{
			"noop",
			func(vmdb vm.StateDB) {
			},
			false,
		},
		{
			"SetState",
			func(vmdb vm.StateDB) {
				vmdb.SetState(addr2, common.BigToHash(big.NewInt(1)), common.BigToHash(big.NewInt(2)))
			},
			false,
		},
		{
			"SetCode",
			func(vmdb vm.StateDB) {
				vmdb.SetCode(addr2, []byte{1, 2, 3})
			},
			false,
		},
		{
			"SetState",
			func(vmdb vm.StateDB) {
				vmdb.SetState(addr2, common.BytesToHash([]byte{1, 2, 3}), common.BytesToHash([]byte{4, 5, 6}))
			},
			false,
		},
		{
			"SetState + SetCode",
			func(vmdb vm.StateDB) {
				vmdb.SetCode(addr1, []byte{10})
				vmdb.SetState(addr2, common.BytesToHash([]byte{1, 2, 3}), common.BytesToHash([]byte{4, 5, 6}))
			},
			false,
		},
		{
			// Fails due to different account numbers due to different SetAccount ordering
			// Journal -> SetAccount ordered by address @ Commit() -> addr1, addr2
			// CacheCtx -> Ordered by first SetCode/SetState call -> addr2, addr1
			"SetState + SetCode, reverse address",
			func(vmdb vm.StateDB) {
				vmdb.SetCode(addr2, []byte{10})
				vmdb.SetState(addr1, common.BytesToHash([]byte{1, 2, 3}), common.BytesToHash([]byte{4, 5, 6}))
			},
			true,
		},
	}

	for _, tt := range tests {
		suite.Run(tt.name, func() {
			if tt.shouldSkip {
				suite.T().Skip("skipping test - state incompatible")
			}

			suite.SetupTest()
			suite.Commit()

			// Cache CTX statedb
			ctxDB := statedb.New(suite.Ctx, suite.App.EvmKeeper, emptyTxConfig)
			tt.maleate(ctxDB)
			suite.Require().NoError(ctxDB.Commit())

			newRes := suite.Commit()

			cacheNodes := suite.exportIAVLStoreNodes(suite.App.GetKey(authtypes.StoreKey))
			cacheHashes := suite.GetStoreHashes()

			// --------------------------------------------
			// Reset state for legacy journal based StateDB
			suite.SetupTest()
			suite.Commit()

			legacyDB := statedb_legacy.New(suite.Ctx, suite.App.EvmKeeper, emptyTxConfig)
			tt.maleate(legacyDB)
			suite.Require().NoError(legacyDB.Commit())

			legacyRes := suite.Commit()

			suite.T().Logf("newRes: %x", newRes.Data)
			suite.T().Logf("legacyRes: %x", legacyRes.Data)

			suite.Equalf(
				common.Bytes2Hex(newRes.Data),
				common.Bytes2Hex(legacyRes.Data),
				"commitID.Hash should match between statedb versions, old %x, new %x",
				legacyRes.Data,
				newRes.Data,
			)

			// Don't log any additional info if the test passed
			if !suite.T().Failed() {
				return
			}

			journalNodes := suite.exportIAVLStoreNodes(suite.App.GetKey(authtypes.StoreKey))
			journalHashes := suite.GetStoreHashes()

			suite.Equal(cacheHashes, journalHashes)

			hashDiff := storeHashDiff(cacheHashes, journalHashes)
			for k, v := range hashDiff {
				suite.T().Logf("%v (cache -> journal): %v", k, v)
			}

			fmt.Printf("----------------------------------------\n")
			fmt.Printf("CTX NODES:\n")
			fmt.Printf("%v\n\n", cacheNodes)
			fmt.Printf("----------------------------------------\n")
			fmt.Printf("JOURNAL NODES:\n")
			fmt.Printf("%v\n\n", journalNodes)
		})
	}
}

func storeHashDiff(
	aHashes map[string]string,
	bHashes map[string]string,
) map[string]string {
	diff := make(map[string]string)

	for k, aHash := range aHashes {
		bHash, ok := bHashes[k]
		if !ok {
			diff[k] = fmt.Sprintf("%v -> nil", aHash)
			continue
		}

		if aHash != bHash {
			diff[k] = fmt.Sprintf("%v -> %v", aHash, bHash)
		}
	}

	for k, bHash := range bHashes {
		if _, ok := aHashes[k]; !ok {
			diff[k] = fmt.Sprintf("nil -> %v", bHash)
		}
	}

	return diff
}

// GetStoreHashes returns the IAVL hashes of all the stores in the multistore
func (suite *KeeperTestSuite) GetStoreHashes() map[string]string {
	cms := suite.App.CommitMultiStore()
	storeKeys := suite.App.GetKeys()
	storeHashes := make(map[string]string)

	for _, key := range storeKeys {
		store := cms.GetStore(key)
		iavlStore := store.(*iavl.Store)
		storeHashes[key.Name()] = common.Bytes2Hex(iavlStore.LastCommitID().Hash)
	}

	return storeHashes
}

func (suite *KeeperTestSuite) exportIAVLStoreNodes(
	storeKey *storetypes.KVStoreKey,
) string {
	cms := suite.App.CommitMultiStore()
	store := cms.GetStore(storeKey)
	authIavlStore := store.(*iavl.Store)

	lastVersion := authIavlStore.LastCommitID().Version

	exporter, err := authIavlStore.Export(lastVersion)
	suite.Require().NoError(err)
	defer exporter.Close()

	nodes := []*cosmosiavl.ExportNode{}
	for {
		node, err := exporter.Next()
		if err != nil || node == nil {
			break
		}

		nodes = append(nodes, node)
	}

	s := ""
	s += fmt.Sprintf("%v store nodes @ version %v\n", len(nodes), lastVersion)

	for _, node := range nodes {
		indent := strings.Repeat(" ", int(node.Height))

		valueStr := fmt.Sprintf("%x", node.Value)

		if len(node.Value) == 0 {
			valueStr = "nil"
		}

		s += fmt.Sprintf(
			"%v[%v-%v] %x -> %s\n",
			indent,
			node.Height,
			node.Version,
			node.Key,
			valueStr,
		)
	}

	return s
}
