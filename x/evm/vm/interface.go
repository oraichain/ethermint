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
package vm

import (
	"math/big"

	storetypes "github.com/cosmos/cosmos-sdk/store/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/evmos/ethermint/x/evm/types"

	"github.com/ethereum/go-ethereum/common"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/core/vm"
	"github.com/ethereum/go-ethereum/params"
	"github.com/holiman/uint256"
)

// PrecompiledContracts defines a map of address -> precompiled contract
type PrecompiledContracts map[common.Address]vm.PrecompiledContract

type StatefulPrecompiledContract interface {
	vm.PrecompiledContract
	RunStateful(evm EVM, addr common.Address, input []byte, value *big.Int) (ret []byte, err error)
}

// EVM defines the interface for the Ethereum Virtual Machine used by the EVM module.
type EVM interface {
	Config() vm.Config
	Context() vm.BlockContext
	TxContext() vm.TxContext

	Reset(txCtx vm.TxContext, statedb vm.StateDB)
	Cancel()
	Cancelled() bool //nolint
	Interpreter() *vm.EVMInterpreter
	Call(caller vm.ContractRef, addr common.Address, input []byte, gas uint64, value *big.Int) (ret []byte, leftOverGas uint64, err error)
	CallCode(caller vm.ContractRef, addr common.Address, input []byte, gas uint64, value *big.Int) (ret []byte, leftOverGas uint64, err error)
	DelegateCall(caller vm.ContractRef, addr common.Address, input []byte, gas uint64) (ret []byte, leftOverGas uint64, err error)
	StaticCall(caller vm.ContractRef, addr common.Address, input []byte, gas uint64) (ret []byte, leftOverGas uint64, err error)
	Create(caller vm.ContractRef, code []byte, gas uint64, value *big.Int) (ret []byte, contractAddr common.Address, leftOverGas uint64, err error)
	Create2(
		caller vm.ContractRef,
		code []byte,
		gas uint64,
		endowment *big.Int,
		salt *uint256.Int) (
		ret []byte, contractAddr common.Address, leftOverGas uint64, err error,
	)
	ChainConfig() *params.ChainConfig

	ActivePrecompiles(rules params.Rules) []common.Address
	Precompile(addr common.Address) (vm.PrecompiledContract, bool)
	RunPrecompiledContract(
		p StatefulPrecompiledContract,
		addr common.Address,
		input []byte,
		suppliedGas uint64,
		value *big.Int) (
		ret []byte, remainingGas uint64, err error,
	)
}

// Constructor defines the function used to instantiate the EVM on
// each state transition.
type Constructor func(
	blockCtx vm.BlockContext,
	txCtx vm.TxContext,
	stateDB vm.StateDB,
	chainConfig *params.ChainConfig,
	config vm.Config,
	customPrecompiles PrecompiledContracts,
) EVM

// StateDBKeeper provide underlying storage of StateDB
type StateDBKeeper interface {
	// Read methods
	GetAccount(ctx sdk.Context, addr common.Address) *types.StateDBAccount
	GetState(ctx sdk.Context, addr common.Address, key common.Hash) common.Hash
	GetCode(ctx sdk.Context, codeHash common.Hash) []byte
	// the callback returns false to break early
	ForEachStorage(ctx sdk.Context, addr common.Address, cb func(key, value common.Hash) bool)

	// Write methods, only called by `StateDB.Commit()`
	SetAccount(ctx sdk.Context, addr common.Address, account types.StateDBAccount) error
	SetState(ctx sdk.Context, addr common.Address, key common.Hash, value []byte)
	SetCode(ctx sdk.Context, codeHash []byte, code []byte)
	SetBalance(ctx sdk.Context, addr common.Address, amount *big.Int) error
	DeleteAccount(ctx sdk.Context, addr common.Address) error

	GetTransientKey() storetypes.StoreKey
}

type StateDB interface {
	vm.StateDB

	Keeper() StateDBKeeper

	// Additional methods required by x/evm Keeper
	Commit() error
	Logs() []*ethtypes.Log
}

// TxConfig provides readonly information of current tx for `StateDB`.
type TxConfig interface {
	BlockHash() common.Hash // hash of current block
	TxHash() common.Hash    // hash of current tx
	TxIndex() uint          // the index of current transaction
	LogIndex() uint         // the index of next log within current block
}

type StateDBConstructor func(
	ctx sdk.Context,
	keeper StateDBKeeper,
	txConfig types.TxConfig,
) StateDB
