package testutil

import (
	// embed compiled smart contract
	_ "embed"
	"encoding/json"
	"math/big"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/evmos/ethermint/server/config"
	"github.com/evmos/ethermint/x/evm/types"
)

var (
	//go:embed StateTest.json
	stateTestContractJSON []byte

	// StateTestContract is a compiled contract for testing state changes
	StateTestContract types.CompiledContract
)

func loadContract(jsonBytes []byte) types.CompiledContract {
	var contract types.CompiledContract
	err := json.Unmarshal(jsonBytes, &contract)
	if err != nil {
		panic(err)
	}

	if len(contract.Bin) == 0 {
		panic("load contract failed")
	}

	return contract
}

func init() {
	StateTestContract = loadContract(stateTestContractJSON)
}

// DeployContract deploys a provided contract and returns the contract address
func (suite *TestSuite) DeployContract(
	contract types.CompiledContract,
	params ...interface{},
) common.Address {
	ctx := sdk.WrapSDKContext(suite.Ctx)
	chainID := suite.App.EvmKeeper.ChainID()

	ctorArgs, err := contract.ABI.Pack("", params...)
	suite.Require().NoError(err)

	nonce := suite.App.EvmKeeper.GetNonce(suite.Ctx, suite.Address)

	data := contract.Bin
	data = append(data, ctorArgs...)
	args, err := json.Marshal(&types.TransactionArgs{
		From: &suite.Address,
		Data: (*hexutil.Bytes)(&data),
	})
	suite.Require().NoError(err)
	res, err := suite.QueryClient.EstimateGas(ctx, &types.EthCallRequest{
		Args:            args,
		GasCap:          config.DefaultGasCap,
		ProposerAddress: suite.Ctx.BlockHeader().ProposerAddress,
	})
	suite.Require().NoError(err)

	var erc20DeployTx *types.MsgEthereumTx
	if suite.EnableFeemarket {
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
	err = erc20DeployTx.Sign(ethtypes.LatestSignerForChainID(chainID), suite.Signer)
	suite.Require().NoError(err)

	rsp, err := suite.App.EvmKeeper.EthereumTx(ctx, erc20DeployTx)
	suite.Require().NoError(err)
	suite.Require().Empty(rsp.VmError)

	return crypto.CreateAddress(suite.Address, nonce)
}

func (suite *TestSuite) CallContract(
	contract types.CompiledContract,
	contractAddr common.Address,
	method string,
	params ...interface{},
) (*types.MsgEthereumTx, *types.MsgEthereumTxResponse, error) {
	ctx := sdk.WrapSDKContext(suite.Ctx)
	chainID := suite.App.EvmKeeper.ChainID()

	transferData, err := contract.ABI.Pack(method, params...)
	if err != nil {
		return nil, nil, err
	}
	args, err := json.Marshal(&types.TransactionArgs{
		To:   &contractAddr,
		From: &suite.Address,
		Data: (*hexutil.Bytes)(&transferData),
	})
	if err != nil {
		return nil, nil, err
	}
	res, err := suite.QueryClient.EstimateGas(ctx, &types.EthCallRequest{
		Args:            args,
		GasCap:          25_000_000,
		ProposerAddress: suite.Ctx.BlockHeader().ProposerAddress,
	})
	if err != nil {
		return nil, nil, err
	}

	nonce := suite.App.EvmKeeper.GetNonce(suite.Ctx, suite.Address)

	var ercTransferTx *types.MsgEthereumTx
	if suite.EnableFeemarket {
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
	err = ercTransferTx.Sign(ethtypes.LatestSignerForChainID(chainID), suite.Signer)
	if err != nil {
		return nil, nil, err
	}
	rsp, err := suite.App.EvmKeeper.EthereumTx(ctx, ercTransferTx)
	if err != nil {
		return nil, rsp, err
	}

	return ercTransferTx, rsp, nil
}
