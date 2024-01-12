// Copyright 2020 The go-ethereum Authors
// This file is part of the go-ethereum library.
//
// The go-ethereum library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The go-ethereum library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the go-ethereum library. If not, see <http://www.gnu.org/licenses/>.

package statedb

import (
	"github.com/cosmos/cosmos-sdk/store/prefix"
	"github.com/ethereum/go-ethereum/common"

	storetypes "github.com/cosmos/cosmos-sdk/store/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
)

type accessList struct {
	key storetypes.StoreKey
}

// ContainsAddress returns true if the address is in the access list.
func (al *accessList) ContainsAddress(ctx sdk.Context, address common.Address) bool {
	store := prefix.NewStore(ctx.KVStore(al.key), AccessListAddressKey)
	return store.Has(address.Bytes())
}

// Contains checks if a slot within an account is present in the access list, returning
// separate flags for the presence of the account and the slot respectively.
func (al *accessList) Contains(
	ctx sdk.Context,
	address common.Address,
	slot common.Hash,
) (addressPresent bool, slotPresent bool) {
	addressPresent = al.ContainsAddress(ctx, address)
	if !addressPresent {
		// no address so no slots
		return false, false
	}

	store := prefix.NewStore(ctx.KVStore(al.key), AccessListAddressSlotKey)
	slotPresent = store.Has(append(address.Bytes(), slot.Bytes()...))

	return true, slotPresent
}

// newAccessList creates a new accessList.
func newAccessList(storeKey storetypes.StoreKey) *accessList {
	return &accessList{
		key: storeKey,
	}
}

// AddAddress adds an address to the access list, and returns 'true' if the operation
// caused a change (addr was not previously in the list).
func (al *accessList) AddAddress(ctx sdk.Context, address common.Address) bool {
	store := prefix.NewStore(ctx.KVStore(al.key), AccessListAddressKey)
	if store.Has(address.Bytes()) {
		return false
	}

	store.Set(address.Bytes(), []byte{})
	return true
}

// AddSlot adds the specified (addr, slot) combo to the access list.
// Return values are:
// - address added
// - slot added
// For any 'true' value returned, a corresponding journal entry must be made.
func (al *accessList) AddSlot(
	ctx sdk.Context,
	address common.Address,
	slot common.Hash,
) (addrChange bool, slotChange bool) {
	// Add address if not present
	addrChange = al.AddAddress(ctx, address)

	// Add slot if not present
	store := prefix.NewStore(ctx.KVStore(al.key), AccessListAddressSlotKey)
	if store.Has(append(address.Bytes(), slot.Bytes()...)) {
		// Already contains the slot
		return addrChange, false
	}

	store.Set(append(address.Bytes(), slot.Bytes()...), []byte{})
	return addrChange, true
}
