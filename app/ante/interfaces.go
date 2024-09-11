package ante

import (
	"math/big"

	cryptotypes "github.com/cosmos/cosmos-sdk/crypto/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	tx "github.com/cosmos/cosmos-sdk/types/tx"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/vm"
	"github.com/ethereum/go-ethereum/params"
	"github.com/tharsis/ethermint/x/evm/statedb"
	evmtypes "github.com/tharsis/ethermint/x/evm/types"
)

// EVMKeeper defines the expected keeper interface used on the Eth AnteHandler
type EVMKeeper interface {
	statedb.Keeper

	ChainID() *big.Int
	GetCosmosAddressMapping(ctx sdk.Context, evmAddress common.Address) sdk.AccAddress
	GetParams(ctx sdk.Context) evmtypes.Params
	NewEVM(ctx sdk.Context, msg core.Message, cfg *evmtypes.EVMConfig, tracer vm.EVMLogger, stateDB vm.StateDB) *vm.EVM
	DeductTxCostsFromUserBalance(
		ctx sdk.Context, msgEthTx evmtypes.MsgEthereumTx, txData evmtypes.TxData, denom string, homestead, istanbul, london bool,
	) (sdk.Coins, error)
	BaseFee(ctx sdk.Context, ethCfg *params.ChainConfig) *big.Int
	GetBalance(ctx sdk.Context, addr common.Address) *big.Int
	ResetTransientGasUsed(ctx sdk.Context)
	ValidateSignerAnte(ctx sdk.Context, pk cryptotypes.PubKey, signer sdk.AccAddress) error
}

type protoTxProvider interface {
	GetProtoTx() *tx.Tx
}
