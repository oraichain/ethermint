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
package statedb

import (
	"bytes"
	"math/big"
	"sort"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/evmos/ethermint/x/evm/types"
)

var emptyCodeHash = crypto.Keccak256(nil)

// Storage represents in-memory cache/buffer of contract storage.
type Storage map[common.Hash]common.Hash

// SortedKeys sort the keys for deterministic iteration
func (s Storage) SortedKeys() []common.Hash {
	keys := make([]common.Hash, len(s))
	i := 0
	for k := range s {
		keys[i] = k
		i++
	}
	sort.Slice(keys, func(i, j int) bool {
		return bytes.Compare(keys[i].Bytes(), keys[j].Bytes()) < 0
	})
	return keys
}

// stateObject is the state of an acount
type stateObject struct {
	db *StateDB

	address common.Address

	suicided bool
}

// newObject creates a state object.
func newObject(db *StateDB, address common.Address, account types.StateDBAccount) *stateObject {
	if account.Balance == nil {
		account.Balance = new(big.Int)
	}
	if account.CodeHash == nil {
		account.CodeHash = emptyCodeHash
	}
	return &stateObject{
		db:      db,
		address: address,
	}
}

func (s *stateObject) markSuicided() {
	s.suicided = true
}
