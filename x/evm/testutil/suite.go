package testutil

import (
	_ "embed"
	"encoding/json"
	"math"
	"math/big"
	"time"

	sdkmath "cosmossdk.io/math"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	"github.com/cosmos/cosmos-sdk/baseapp"
	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/codec"
	"github.com/cosmos/cosmos-sdk/crypto/keyring"
	"github.com/cosmos/cosmos-sdk/simapp"
	sdk "github.com/cosmos/cosmos-sdk/types"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"
	feemarkettypes "github.com/evmos/ethermint/x/feemarket/types"
	tmjson "github.com/tendermint/tendermint/libs/json"

	"github.com/evmos/ethermint/app"
	"github.com/evmos/ethermint/crypto/ethsecp256k1"
	"github.com/evmos/ethermint/encoding"
	"github.com/evmos/ethermint/server/config"
	"github.com/evmos/ethermint/tests"
	ethermint "github.com/evmos/ethermint/types"
	"github.com/evmos/ethermint/x/evm/statedb"
	"github.com/evmos/ethermint/x/evm/types"
	evmtypes "github.com/evmos/ethermint/x/evm/types"
	"github.com/evmos/ethermint/x/evm/vm"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/params"

	abci "github.com/tendermint/tendermint/abci/types"
	"github.com/tendermint/tendermint/crypto/tmhash"
	tmproto "github.com/tendermint/tendermint/proto/tendermint/types"
	tmversion "github.com/tendermint/tendermint/proto/tendermint/version"
	"github.com/tendermint/tendermint/version"
)

var testTokens = sdkmath.NewIntWithDecimal(1000, 18)

type KeeperTestSuite struct {
	suite.Suite

	Ctx         sdk.Context
	App         *app.EthermintApp
	QueryClient types.QueryClient
	Address     common.Address
	ConsAddress sdk.ConsAddress

	// for generate test tx
	ClientCtx client.Context
	EthSigner ethtypes.Signer

	appCodec codec.Codec
	signer   keyring.Signer

	enableFeemarket  bool
	enableLondonHF   bool
	mintFeeCollector bool
	denom            string
}

var s *KeeperTestSuite

func (suite *KeeperTestSuite) SetupTest() {
	checkTx := false
	suite.App = app.Setup(checkTx, nil)
	suite.SetupApp(checkTx)
}

func (suite *KeeperTestSuite) SetupTestWithT(t require.TestingT) {
	checkTx := false
	suite.App = app.Setup(checkTx, nil)
	suite.SetupAppWithT(checkTx, t)
}

func (suite *KeeperTestSuite) SetupApp(checkTx bool) {
	suite.SetupAppWithT(checkTx, suite.T())
}

// SetupApp setup test environment, it uses`require.TestingT` to support both `testing.T` and `testing.B`.
func (suite *KeeperTestSuite) SetupAppWithT(checkTx bool, t require.TestingT) {
	// account key, use a constant account to keep unit test deterministic.
	ecdsaPriv, err := crypto.HexToECDSA("b71c71a67e1177ad4e901695e1b4b9ee17ae16c6668d313eac2f96dbcda3f291")
	require.NoError(t, err)
	priv := &ethsecp256k1.PrivKey{
		Key: crypto.FromECDSA(ecdsaPriv),
	}
	suite.Address = common.BytesToAddress(priv.PubKey().Address().Bytes())
	suite.signer = tests.NewSigner(priv)

	// consensus key
	priv, err = ethsecp256k1.GenerateKey()
	require.NoError(t, err)
	suite.ConsAddress = sdk.ConsAddress(priv.PubKey().Address())

	suite.App = app.Setup(checkTx, func(app *app.EthermintApp, genesis simapp.GenesisState) simapp.GenesisState {
		feemarketGenesis := feemarkettypes.DefaultGenesisState()
		if suite.enableFeemarket {
			feemarketGenesis.Params.EnableHeight = 1
			feemarketGenesis.Params.NoBaseFee = false
		} else {
			feemarketGenesis.Params.NoBaseFee = true
		}
		genesis[feemarkettypes.ModuleName] = app.AppCodec().MustMarshalJSON(feemarketGenesis)
		if !suite.enableLondonHF {
			evmGenesis := types.DefaultGenesisState()
			maxInt := sdkmath.NewInt(math.MaxInt64)
			evmGenesis.Params.ChainConfig.LondonBlock = &maxInt
			evmGenesis.Params.ChainConfig.ArrowGlacierBlock = &maxInt
			evmGenesis.Params.ChainConfig.GrayGlacierBlock = &maxInt
			evmGenesis.Params.ChainConfig.MergeNetsplitBlock = &maxInt
			evmGenesis.Params.ChainConfig.ShanghaiBlock = &maxInt
			evmGenesis.Params.ChainConfig.CancunBlock = &maxInt
			genesis[types.ModuleName] = app.AppCodec().MustMarshalJSON(evmGenesis)
		}
		return genesis
	})

	if suite.mintFeeCollector {
		// mint some coin to fee collector
		coins := sdk.NewCoins(sdk.NewCoin(types.DefaultEVMDenom, sdkmath.NewInt(int64(params.TxGas)-1)))
		genesisState := app.NewTestGenesisState(suite.App.AppCodec())
		balances := []banktypes.Balance{
			{
				Address: suite.App.AccountKeeper.GetModuleAddress(authtypes.FeeCollectorName).String(),
				Coins:   coins,
			},
		}
		var bankGenesis banktypes.GenesisState
		suite.App.AppCodec().MustUnmarshalJSON(genesisState[banktypes.ModuleName], &bankGenesis)
		// Update balances and total supply
		bankGenesis.Balances = append(bankGenesis.Balances, balances...)
		bankGenesis.Supply = bankGenesis.Supply.Add(coins...)
		genesisState[banktypes.ModuleName] = suite.App.AppCodec().MustMarshalJSON(&bankGenesis)

		// we marshal the genesisState of all module to a byte array
		stateBytes, err := tmjson.MarshalIndent(genesisState, "", " ")
		require.NoError(t, err)

		// Initialize the chain
		suite.App.InitChain(
			abci.RequestInitChain{
				ChainId:         "ethermint_9000-1",
				Validators:      []abci.ValidatorUpdate{},
				ConsensusParams: app.DefaultConsensusParams,
				AppStateBytes:   stateBytes,
			},
		)
	}

	suite.Ctx = suite.App.BaseApp.NewContext(checkTx, tmproto.Header{
		Height:          1,
		ChainID:         "ethermint_9000-1",
		Time:            time.Now().UTC(),
		ProposerAddress: suite.ConsAddress.Bytes(),
		Version: tmversion.Consensus{
			Block: version.BlockProtocol,
		},
		LastBlockId: tmproto.BlockID{
			Hash: tmhash.Sum([]byte("block_id")),
			PartSetHeader: tmproto.PartSetHeader{
				Total: 11,
				Hash:  tmhash.Sum([]byte("partset_header")),
			},
		},
		AppHash:            tmhash.Sum([]byte("app")),
		DataHash:           tmhash.Sum([]byte("data")),
		EvidenceHash:       tmhash.Sum([]byte("evidence")),
		ValidatorsHash:     tmhash.Sum([]byte("validators")),
		NextValidatorsHash: tmhash.Sum([]byte("next_validators")),
		ConsensusHash:      tmhash.Sum([]byte("consensus")),
		LastResultsHash:    tmhash.Sum([]byte("last_result")),
	})

	queryHelper := baseapp.NewQueryServerTestHelper(suite.Ctx, suite.App.InterfaceRegistry())
	types.RegisterQueryServer(queryHelper, suite.App.EvmKeeper)
	suite.QueryClient = types.NewQueryClient(queryHelper)

	acc := &ethermint.EthAccount{
		BaseAccount: authtypes.NewBaseAccount(sdk.AccAddress(suite.Address.Bytes()), nil, 0, 0),
		CodeHash:    common.BytesToHash(crypto.Keccak256(nil)).String(),
	}

	suite.App.AccountKeeper.SetAccount(suite.Ctx, acc)

	valAddr := sdk.ValAddress(suite.Address.Bytes())
	validator, err := stakingtypes.NewValidator(valAddr, priv.PubKey(), stakingtypes.Description{})
	require.NoError(t, err)
	err = suite.App.StakingKeeper.SetValidatorByConsAddr(suite.Ctx, validator)
	require.NoError(t, err)
	err = suite.App.StakingKeeper.SetValidatorByConsAddr(suite.Ctx, validator)
	require.NoError(t, err)
	suite.App.StakingKeeper.SetValidator(suite.Ctx, validator)

	encodingConfig := encoding.MakeConfig(app.ModuleBasics)
	suite.ClientCtx = client.Context{}.WithTxConfig(encodingConfig.TxConfig)
	suite.EthSigner = ethtypes.LatestSignerForChainID(suite.App.EvmKeeper.ChainID())
	suite.appCodec = encodingConfig.Codec
	suite.denom = evmtypes.DefaultEVMDenom
}

func (suite *KeeperTestSuite) EvmDenom() string {
	ctx := sdk.WrapSDKContext(suite.Ctx)
	rsp, _ := suite.QueryClient.Params(ctx, &types.QueryParamsRequest{})
	return rsp.Params.EvmDenom
}

// Commit and begin new block
func (suite *KeeperTestSuite) Commit() {
	_ = suite.App.Commit()
	header := suite.Ctx.BlockHeader()
	header.Height += 1
	suite.App.BeginBlock(abci.RequestBeginBlock{
		Header: header,
	})

	// update ctx
	suite.Ctx = suite.App.BaseApp.NewContext(false, header)

	queryHelper := baseapp.NewQueryServerTestHelper(suite.Ctx, suite.App.InterfaceRegistry())
	types.RegisterQueryServer(queryHelper, suite.App.EvmKeeper)
	suite.QueryClient = types.NewQueryClient(queryHelper)
}

func (suite *KeeperTestSuite) StateDB() vm.StateDB {
	return statedb.New(
		suite.Ctx,
		suite.App.EvmKeeper,
		types.NewEmptyTxConfig(common.BytesToHash(suite.Ctx.HeaderHash().Bytes())),
	)
}

// DeployTestContract deploy a test erc20 contract and returns the contract address
func (suite *KeeperTestSuite) DeployTestContract(t require.TestingT, owner common.Address, supply *big.Int) common.Address {
	ctx := sdk.WrapSDKContext(suite.Ctx)
	chainID := suite.App.EvmKeeper.ChainID()

	ctorArgs, err := types.ERC20Contract.ABI.Pack("", owner, supply)
	require.NoError(t, err)

	nonce := suite.App.EvmKeeper.GetNonce(suite.Ctx, suite.Address)

	data := append(types.ERC20Contract.Bin, ctorArgs...)
	args, err := json.Marshal(&types.TransactionArgs{
		From: &suite.Address,
		Data: (*hexutil.Bytes)(&data),
	})
	require.NoError(t, err)
	res, err := suite.QueryClient.EstimateGas(ctx, &types.EthCallRequest{
		Args:            args,
		GasCap:          uint64(config.DefaultGasCap),
		ProposerAddress: suite.Ctx.BlockHeader().ProposerAddress,
	})
	require.NoError(t, err)

	var erc20DeployTx *types.MsgEthereumTx
	if suite.enableFeemarket {
		erc20DeployTx = types.NewTxContract(
			chainID,
			nonce,
			nil,     // amount
			res.Gas, // gasLimit
			nil,     // gasPrice
			suite.App.FeeMarketKeeper.GetBaseFee(suite.Ctx),
			big.NewInt(1),
			data,                   // input
			&ethtypes.AccessList{}, // accesses
		)
	} else {
		erc20DeployTx = types.NewTxContract(
			chainID,
			nonce,
			nil,     // amount
			res.Gas, // gasLimit
			nil,     // gasPrice
			nil, nil,
			data, // input
			nil,  // accesses
		)
	}

	erc20DeployTx.From = suite.Address.Hex()
	err = erc20DeployTx.Sign(ethtypes.LatestSignerForChainID(chainID), suite.signer)
	require.NoError(t, err)
	rsp, err := suite.App.EvmKeeper.EthereumTx(ctx, erc20DeployTx)
	require.NoError(t, err)
	require.Empty(t, rsp.VmError)
	return crypto.CreateAddress(suite.Address, nonce)
}

func (suite *KeeperTestSuite) TransferERC20Token(t require.TestingT, contractAddr, from, to common.Address, amount *big.Int) *types.MsgEthereumTx {
	ctx := sdk.WrapSDKContext(suite.Ctx)
	chainID := suite.App.EvmKeeper.ChainID()

	transferData, err := types.ERC20Contract.ABI.Pack("transfer", to, amount)
	require.NoError(t, err)
	args, err := json.Marshal(&types.TransactionArgs{To: &contractAddr, From: &from, Data: (*hexutil.Bytes)(&transferData)})
	require.NoError(t, err)
	res, err := suite.QueryClient.EstimateGas(ctx, &types.EthCallRequest{
		Args:            args,
		GasCap:          25_000_000,
		ProposerAddress: suite.Ctx.BlockHeader().ProposerAddress,
	})
	require.NoError(t, err)

	nonce := suite.App.EvmKeeper.GetNonce(suite.Ctx, suite.Address)

	var ercTransferTx *types.MsgEthereumTx
	if suite.enableFeemarket {
		ercTransferTx = types.NewTx(
			chainID,
			nonce,
			&contractAddr,
			nil,
			res.Gas,
			nil,
			suite.App.FeeMarketKeeper.GetBaseFee(suite.Ctx),
			big.NewInt(1),
			transferData,
			&ethtypes.AccessList{}, // accesses
		)
	} else {
		ercTransferTx = types.NewTx(
			chainID,
			nonce,
			&contractAddr,
			nil,
			res.Gas,
			nil,
			nil, nil,
			transferData,
			nil,
		)
	}

	ercTransferTx.From = suite.Address.Hex()
	err = ercTransferTx.Sign(ethtypes.LatestSignerForChainID(chainID), suite.signer)
	require.NoError(t, err)
	rsp, err := suite.App.EvmKeeper.EthereumTx(ctx, ercTransferTx)
	require.NoError(t, err)
	require.Empty(t, rsp.VmError)
	return ercTransferTx
}

// DeployTestMessageCall deploy a test erc20 contract and returns the contract address
func (suite *KeeperTestSuite) DeployTestMessageCall(t require.TestingT) common.Address {
	ctx := sdk.WrapSDKContext(suite.Ctx)
	chainID := suite.App.EvmKeeper.ChainID()

	data := types.TestMessageCall.Bin
	args, err := json.Marshal(&types.TransactionArgs{
		From: &suite.Address,
		Data: (*hexutil.Bytes)(&data),
	})
	require.NoError(t, err)

	res, err := suite.QueryClient.EstimateGas(ctx, &types.EthCallRequest{
		Args:            args,
		GasCap:          uint64(config.DefaultGasCap),
		ProposerAddress: suite.Ctx.BlockHeader().ProposerAddress,
	})
	require.NoError(t, err)

	nonce := suite.App.EvmKeeper.GetNonce(suite.Ctx, suite.Address)

	var erc20DeployTx *types.MsgEthereumTx
	if suite.enableFeemarket {
		erc20DeployTx = types.NewTxContract(
			chainID,
			nonce,
			nil,     // amount
			res.Gas, // gasLimit
			nil,     // gasPrice
			suite.App.FeeMarketKeeper.GetBaseFee(suite.Ctx),
			big.NewInt(1),
			data,                   // input
			&ethtypes.AccessList{}, // accesses
		)
	} else {
		erc20DeployTx = types.NewTxContract(
			chainID,
			nonce,
			nil,     // amount
			res.Gas, // gasLimit
			nil,     // gasPrice
			nil, nil,
			data, // input
			nil,  // accesses
		)
	}

	erc20DeployTx.From = suite.Address.Hex()
	err = erc20DeployTx.Sign(ethtypes.LatestSignerForChainID(chainID), suite.signer)
	require.NoError(t, err)
	rsp, err := suite.App.EvmKeeper.EthereumTx(ctx, erc20DeployTx)
	require.NoError(t, err)
	require.Empty(t, rsp.VmError)
	return crypto.CreateAddress(suite.Address, nonce)
}

func (suite *KeeperTestSuite) GetAllAccountStorage(
	ctx sdk.Context,
	addr common.Address,
) map[common.Hash]common.Hash {
	states := make(map[common.Hash]common.Hash)
	suite.App.EvmKeeper.ForEachStorage(ctx, addr, func(key, value common.Hash) bool {
		states[key] = value
		// Iterate all
		return true
	})

	return states
}
