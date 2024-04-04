// Copyright 2021 Evmos Foundation
// This file is part of Evmos' Ethermint library.
//
// The Ethermint library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The Ethermint library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the Ethermint library. If not, see https://github.com/evmos/ethermint/blob/main/LICENSE
package keeper

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"math/big"
	"sort"

	sdkmath "cosmossdk.io/math"

	errorsmod "cosmossdk.io/errors"
	"github.com/cosmos/cosmos-sdk/store/cachekv"
	"github.com/cosmos/cosmos-sdk/store/gaskv"
	"github.com/cosmos/cosmos-sdk/store/prefix"
	storetypes "github.com/cosmos/cosmos-sdk/store/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/address"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
	"github.com/ethereum/go-ethereum/common"
	ethermint "github.com/evmos/ethermint/types"
	"github.com/evmos/ethermint/x/evm/legacystatedb"
	"github.com/evmos/ethermint/x/evm/statedb"
	"github.com/evmos/ethermint/x/evm/types"
)

var _ statedb.Keeper = &Keeper{}

// ----------------------------------------------------------------------------
// StateDB Keeper implementation
// ----------------------------------------------------------------------------

// GetAccount returns nil if account is not exist, returns error if it's not `EthAccountI`
func (k *Keeper) GetAccount(ctx sdk.Context, addr common.Address) *statedb.Account {
	acct := k.GetAccountWithoutBalance(ctx, addr)
	if acct == nil {
		return nil
	}

	// acct.Balance = k.GetBalance(ctx, addr)
	return acct
}

func (k *Keeper) GetAccountLegacy(ctx sdk.Context, addr common.Address) *legacystatedb.Account {
	acct := k.GetAccountWithoutBalance(ctx, addr)
	if acct == nil {
		return nil
	}

	bal := k.GetBalance(ctx, addr)

	legacyAcct := &legacystatedb.Account{
		Nonce:    acct.Nonce,
		Balance:  bal,
		CodeHash: acct.CodeHash,
	}

	return legacyAcct
}

// GetState loads contract state from database, implements `statedb.Keeper` interface.
func (k *Keeper) GetState(ctx sdk.Context, addr common.Address, key common.Hash) common.Hash {
	store := prefix.NewStore(ctx.KVStore(k.storeKey), types.AddressStoragePrefix(addr))

	value := store.Get(key.Bytes())
	if len(value) == 0 {
		return common.Hash{}
	}

	return common.BytesToHash(value)
}

// GetCode loads contract code from database, implements `statedb.Keeper` interface.
func (k *Keeper) GetCode(ctx sdk.Context, codeHash common.Hash) []byte {
	store := prefix.NewStore(ctx.KVStore(k.storeKey), types.KeyPrefixCode)
	return store.Get(codeHash.Bytes())
}

// ForEachStorage iterate contract storage, callback return false to break early
func (k *Keeper) ForEachStorage(ctx sdk.Context, addr common.Address, cb func(key, value common.Hash) bool) {
	store := ctx.KVStore(k.storeKey)
	prefix := types.AddressStoragePrefix(addr)

	iterator := sdk.KVStorePrefixIterator(store, prefix)
	defer iterator.Close()

	for ; iterator.Valid(); iterator.Next() {
		key := common.BytesToHash(iterator.Key())
		value := common.BytesToHash(iterator.Value())

		// check if iteration stops
		if !cb(key, value) {
			return
		}
	}
}

// SetBalance update account's balance, compare with current balance first, then decide to mint or burn.
func (k *Keeper) SetBalance(ctx sdk.Context, addr common.Address, amount *big.Int) error {
	cosmosAddr := sdk.AccAddress(addr.Bytes())

	params := k.GetParams(ctx)
	coin := k.bankKeeper.GetBalance(ctx, cosmosAddr, params.EvmDenom)
	balance := coin.Amount.BigInt()
	delta := new(big.Int).Sub(amount, balance)
	switch delta.Sign() {
	case 1:
		// mint
		coins := sdk.NewCoins(sdk.NewCoin(params.EvmDenom, sdkmath.NewIntFromBigInt(delta)))
		if err := k.bankKeeper.MintCoins(ctx, types.ModuleName, coins); err != nil {
			return err
		}
		if err := k.bankKeeper.SendCoinsFromModuleToAccount(ctx, types.ModuleName, cosmosAddr, coins); err != nil {
			return err
		}
	case -1:
		// burn
		coins := sdk.NewCoins(sdk.NewCoin(params.EvmDenom, sdkmath.NewIntFromBigInt(new(big.Int).Neg(delta))))
		if err := k.bankKeeper.SendCoinsFromAccountToModule(ctx, cosmosAddr, types.ModuleName, coins); err != nil {
			return err
		}
		if err := k.bankKeeper.BurnCoins(ctx, types.ModuleName, coins); err != nil {
			return err
		}
	default:
		// not changed
	}
	return nil
}

func (k *Keeper) SetAccountLegacy(ctx sdk.Context, addr common.Address, account legacystatedb.Account) error {
	k.SetAccount(ctx, addr, statedb.Account{
		Nonce:         account.Nonce,
		CodeHash:      account.CodeHash,
		AccountNumber: 0,
	})

	return k.SetBalance(ctx, addr, account.Balance)
}

// SetAccount updates nonce/balance/codeHash together.
func (k *Keeper) SetAccount(ctx sdk.Context, addr common.Address, account statedb.Account) error {
	// update account
	cosmosAddr := sdk.AccAddress(addr.Bytes())
	acct := k.accountKeeper.GetAccount(ctx, cosmosAddr)
	if acct == nil {
		acct = k.accountKeeper.NewAccountWithAddress(ctx, cosmosAddr)
	}

	if err := acct.SetSequence(account.Nonce); err != nil {
		return err
	}

	codeHash := common.BytesToHash(account.CodeHash)
	ethAcct, ok := acct.(ethermint.EthAccountI)

	if ok {
		if err := ethAcct.SetCodeHash(codeHash); err != nil {
			return err
		}

		if account.AccountNumber != 0 {
			if err := ethAcct.SetAccountNumber(account.AccountNumber); err != nil {
				return err
			}
		}
	}

	if !ok && account.IsContract() {
		if baseAcct, isBaseAccount := acct.(*authtypes.BaseAccount); isBaseAccount {
			acct = &ethermint.EthAccount{
				BaseAccount: baseAcct,
				CodeHash:    codeHash.Hex(),
			}
		} else {
			return errorsmod.Wrapf(types.ErrInvalidAccount, "type %T, address %s", acct, addr)
		}
	}

	k.accountKeeper.SetAccount(ctx, acct)

	k.Logger(ctx).Debug(
		"account updated",
		"ethereum-address", addr.Hex(),
		"nonce", account.Nonce,
		"codeHash", codeHash.Hex(),
		// "balance", account.Balance,
	)
	return nil
}

func getCacheKVStore(ctx sdk.Context, storeKey storetypes.StoreKey) (*cachekv.Store, error) {
	ctxStore := ctx.KVStore(storeKey)
	gasKVStore, ok := ctxStore.(*gaskv.Store)
	if !ok {
		return nil, fmt.Errorf("expected gaskv.Store, got %T", ctxStore)
	}

	// Use parent of store and try as cachekv.Store
	ctxStore = gasKVStore.GetParent()

	cacheKVStore, ok := ctxStore.(*cachekv.Store)
	if !ok {
		return nil, fmt.Errorf("expected cachekv.Store, got %T", ctxStore)
	}

	return cacheKVStore, nil
}

func (k *Keeper) UnsetBalanceChange(ctx sdk.Context, addr common.Address) error {
	bankStore, err := getCacheKVStore(ctx, k.bankStoreKey)
	if err != nil {
		return err
	}

	// We don't use params.EvmDenom since EvmDenom isn't used with bank module
	// but x/evmutil only. We use ukava instead which is managed by x/evmutil
	// and is the true denom being modified in x/bank
	kavaDenom := "ukava"

	// Stores modified by bankkeeper.setBalance must be unset
	cosmosAddr := sdk.AccAddress(addr.Bytes())
	// Equivalent to using prefix.NewStore(cacheKVStore, banktypes.CreateAccountBalancesPrefix(cosmosAddr))
	// but Unset is only defined on cachekv.Store
	accKey := append(banktypes.CreateAccountBalancesPrefix(cosmosAddr), []byte(kavaDenom)...)
	bankStore.Unset(accKey)

	denomAddrKey := address.MustLengthPrefix(cosmosAddr)
	denomKey := append(banktypes.CreateDenomAddressPrefix(kavaDenom), denomAddrKey...)
	bankStore.Unset(denomKey)

	evmutilStore, err := getCacheKVStore(ctx, k.evmutilStoreKey)
	if err != nil {
		return err
	}

	key := append([]byte{0x00}, address.MustLengthPrefix(cosmosAddr)...)
	evmutilStore.Unset(key)

	k.Logger(ctx).Info(
		"balance unset",
		"ethereum-address", addr.Hex(),
		"key", hex.EncodeToString(accKey),
		"denom-key", hex.EncodeToString(denomKey),
		"evmutil-key", hex.EncodeToString(key),
	)

	return nil
}

func (k *Keeper) UnsetBankDenomMapping(ctx sdk.Context, addr common.Address) error {
	bankStore, err := getCacheKVStore(ctx, k.bankStoreKey)
	if err != nil {
		return err
	}

	// We don't use params.EvmDenom since EvmDenom isn't used with bank module
	// but x/evmutil only. We use ukava instead which is managed by x/evmutil
	// and is the true denom being modified in x/bank
	kavaDenom := "ukava"

	denomAddrKey := address.MustLengthPrefix(sdk.AccAddress(addr.Bytes()))
	denomKey := append(banktypes.CreateDenomAddressPrefix(kavaDenom), denomAddrKey...)
	bankStore.Unset(denomKey)

	k.Logger(ctx).Info(
		"bank denom mapping unset",
		"ethereum-address", addr.Hex(),
		"denom-key", hex.EncodeToString(denomKey),
	)

	return nil
}

func (k *Keeper) HasBankDenom(ctx sdk.Context, addr common.Address) bool {
	denomPrefixStore := k.getDenomAddressPrefixStore(ctx, "ukava")
	denomAddrKey := address.MustLengthPrefix(sdk.AccAddress(addr.Bytes()))
	return denomPrefixStore.Has(denomAddrKey)
}

func (k *Keeper) getDenomAddressPrefixStore(ctx sdk.Context, denom string) prefix.Store {
	return prefix.NewStore(ctx.KVStore(k.bankStoreKey), banktypes.CreateDenomAddressPrefix(denom))
}

func (k *Keeper) GetAccountNumber(ctx sdk.Context, addr common.Address) (uint64, bool) {
	cosmosAddr := sdk.AccAddress(addr.Bytes())
	acct := k.accountKeeper.GetAccount(ctx, cosmosAddr)
	if acct == nil {
		return 0, false
	}

	return acct.GetAccountNumber(), true
}

// ReassignAccountNumbers reassign account numbers for the given addresses,
// ensuring they are incrementing in order by address and that there are no
// account number gaps, e.g. Accounts that have been created in side effects
// during a transaction and not passed to this method.
func (k *Keeper) ReassignAccountNumbers(ctx sdk.Context, addrs []common.Address) error {
	if len(addrs) == 0 {
		return nil
	}

	// Get all the accounts
	accounts := make([]authtypes.AccountI, len(addrs))
	for i, addr := range addrs {
		cosmosAddr := sdk.AccAddress(addr.Bytes())
		acct := k.accountKeeper.GetAccount(ctx, cosmosAddr)
		if acct == nil {
			return fmt.Errorf("account not found: %s", addr)
		}

		accounts[i] = acct
	}

	// Sort accounts by account number
	sort.Slice(accounts, func(i, j int) bool {
		return accounts[i].GetAccountNumber() < accounts[j].GetAccountNumber()
	})

	// Min account number as start
	accountNumberStart := accounts[0].GetAccountNumber()

	// Ensure there are no gaps in account numbers
	for i, acct := range accounts {
		if acct.GetAccountNumber() != accountNumberStart+uint64(i) {
			return fmt.Errorf(
				"account number mismatch: expected %d, got %d",
				accountNumberStart+uint64(i), acct.GetAccountNumber(),
			)
		}
	}

	// Sort accounts by address
	sort.Slice(accounts, func(i, j int) bool {
		return bytes.Compare(accounts[i].GetAddress(), accounts[j].GetAddress()) < 0
	})

	// Reassign account numbers in order of account address
	for i, acct := range accounts {
		ethAcct, ok := acct.(ethermint.EthAccountI)
		if !ok {
			return fmt.Errorf("invalid account type: %T", acct)
		}

		// set account number
		ethAcct.SetAccountNumber(accountNumberStart + uint64(i))
		k.accountKeeper.SetAccount(ctx, acct)
	}

	return nil
}

// SetState update contract storage, delete if value is empty.
func (k *Keeper) SetState(ctx sdk.Context, addr common.Address, key common.Hash, value []byte) {
	store := prefix.NewStore(ctx.KVStore(k.storeKey), types.AddressStoragePrefix(addr))
	action := "updated"

	// value is a common.Hash which will never be empty as it is always
	// HashLength. The fix is not included here to preserve the original state
	// behavior which will set empty hash values.
	// TODO: Replace value parameter type with common.Hash and replace this
	// checkwith a check for default hash.
	if len(value) == 0 {
		store.Delete(key.Bytes())
		action = "deleted"
	} else {
		store.Set(key.Bytes(), value)
	}
	k.Logger(ctx).Debug(
		fmt.Sprintf("state %s", action),
		"ethereum-address", addr.Hex(),
		"key", key.Hex(),
	)
}

/*
func (k *Keeper) UnsetState(ctx sdk.Context, addr common.Address, key common.Hash) error {
	ctxStore := ctx.KVStore(k.storeKey)
	gasKVStore, ok := ctxStore.(*gaskv.Store)
	if !ok {
		return fmt.Errorf("expected gaskv.Store, got %T", ctxStore)
	}

	// Use parent of store and try as cachekv.Store
	ctxStore = gasKVStore.GetParent()

	cacheKVStore, ok := ctxStore.(*cachekv.Store)
	if !ok {
		return fmt.Errorf("expected cachekv.Store, got %T", ctxStore)
	}

	storeKey := types.AddressStoragePrefix(addr)
	storeKey = append(storeKey, key.Bytes()...)

	cacheKVStore.Unset(storeKey)

	k.Logger(ctx).Debug(
		"state unset",
		"ethereum-address", addr.Hex(),
		"key", key.Hex(),
	)

	return nil
}
*/

// SetCode set contract code, delete if code is empty.
func (k *Keeper) SetCode(ctx sdk.Context, codeHash, code []byte) {
	store := prefix.NewStore(ctx.KVStore(k.storeKey), types.KeyPrefixCode)

	// store or delete code
	action := "updated"
	if len(code) == 0 {
		store.Delete(codeHash)
		action = "deleted"
	} else {
		store.Set(codeHash, code)
	}
	k.Logger(ctx).Debug(
		fmt.Sprintf("code %s", action),
		"code-hash", common.BytesToHash(codeHash).Hex(),
	)
}

// DeleteAccount handles contract's suicide call:
// - clear balance
// - remove code
// - remove states
// - remove auth account
func (k *Keeper) DeleteAccount(ctx sdk.Context, addr common.Address) error {
	cosmosAddr := sdk.AccAddress(addr.Bytes())
	acct := k.accountKeeper.GetAccount(ctx, cosmosAddr)
	if acct == nil {
		return nil
	}

	// NOTE: only Ethereum accounts (contracts) can be selfdestructed
	_, ok := acct.(ethermint.EthAccountI)
	if !ok {
		return errorsmod.Wrapf(types.ErrInvalidAccount, "type %T, address %s", acct, addr)
	}

	// clear balance
	if err := k.SetBalance(ctx, addr, new(big.Int)); err != nil {
		return err
	}

	// clear storage
	k.ForEachStorage(ctx, addr, func(key, _ common.Hash) bool {
		k.SetState(ctx, addr, key, nil)
		return true
	})

	// remove auth account
	k.accountKeeper.RemoveAccount(ctx, acct)

	k.Logger(ctx).Debug(
		"account suicided",
		"ethereum-address", addr.Hex(),
		"cosmos-address", cosmosAddr.String(),
	)

	return nil
}
