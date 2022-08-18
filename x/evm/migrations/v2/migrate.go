package v2

import (
	sdk "github.com/cosmos/cosmos-sdk/types"
	paramtypes "github.com/cosmos/cosmos-sdk/x/params/types"
	"github.com/tharsis/ethermint/x/evm/types"
)

// MigrateStore sets the default AllowUnprotectedTxs parameter.
func MigrateStore(ctx sdk.Context, paramstore *paramtypes.Subspace) error {
	if !paramstore.HasKeyTable() {
		ps := paramstore.WithKeyTable(types.ParamKeyTable())
		paramstore = &ps
	}

	// add msgs to whitelist
	allowedMsgs := []types.EIP712AllowedMsg{
		{
			MsgTypeUrl:       "/kava.evmutil.v1beta1.MsgConvertERC20ToCoin",
			MsgValueTypeName: "MsgValueEVMConvertERC20ToCoin",
			ValueTypes: []types.EIP712MsgAttrType{
				{Name: "initiator", Type: "string"},
				{Name: "receiver", Type: "string"},
				{Name: "kava_erc20_address", Type: "string"},
				{Name: "amount", Type: "string"},
			},
		},
		{
			MsgTypeUrl:       "/kava.evmutil.v1beta1.MsgConvertCoinToERC20",
			MsgValueTypeName: "MsgValueEVMConvertCoinToERC20",
			ValueTypes: []types.EIP712MsgAttrType{
				{Name: "initiator", Type: "string"},
				{Name: "receiver", Type: "string"},
				{Name: "amount", Type: "Coin"},
			},
		},
		{
			MsgTypeUrl:       "/kava.earn.v1beta1.MsgDeposit",
			MsgValueTypeName: "MsgValueEarnDeposit",
			ValueTypes: []types.EIP712MsgAttrType{
				{Name: "depositor", Type: "string"},
				{Name: "amount", Type: "Coin"},
				{Name: "strategy", Type: "number"},
			},
		},
		{
			MsgTypeUrl:       "/kava.evmutil.v1beta1.MsgWithdraw",
			MsgValueTypeName: "MsgValueEarnWithdraw",
			ValueTypes: []types.EIP712MsgAttrType{
				{Name: "from", Type: "string"},
				{Name: "amount", Type: "Coin"},
				{Name: "strategy", Type: "number"},
			},
		},
	}
	paramstore.Set(ctx, types.ParamStoreKeyEIP712AllowedMsgs, allowedMsgs)
	return nil
}
