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
	"fmt"
	"math/big"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/ethereum/go-ethereum/common"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/core/vm"
	"github.com/ethereum/go-ethereum/crypto"
)

var _ vm.StateDB = &StateDB{}

// StateDB structs within the ethereum protocol are used to store anything
// within the merkle trie. StateDBs take care of caching and storing
// nested states. It's the general query interface to retrieve:
// * Contracts
// * Accounts
type StateDB struct {
	keeper   Keeper
	txConfig TxConfig

	ctx            *SnapshotCommitCtx // snapshot-able ctx manager
	ephemeralStore *EphemeralStore    // in-memory temporary data

	// Journal is currently only used for tracking accessList
	journal    *journal
	accessList *accessList

	sdkError error
}

// New creates a new state from a given trie.
func New(ctx sdk.Context, keeper Keeper, txConfig TxConfig) *StateDB {
	return &StateDB{
		keeper:   keeper,
		txConfig: txConfig,

		// This internally creates a branched ctx so calling Commit() is required
		// to write state to the incoming ctx.
		ctx:            NewSnapshotCtx(ctx),
		ephemeralStore: NewEphemeralStore(),

		journal:    newJournal(),
		accessList: newAccessList(),

		sdkError: nil,
	}
}

// Keeper returns the underlying `Keeper`
func (s *StateDB) Keeper() Keeper {
	return s.keeper
}

// AddLog adds a log, called by evm.
func (s *StateDB) AddLog(log *ethtypes.Log) {
	log.TxHash = s.txConfig.TxHash
	log.BlockHash = s.txConfig.BlockHash
	log.TxIndex = s.txConfig.TxIndex
	log.Index = s.txConfig.LogIndex + uint(len(s.ephemeralStore.Logs))

	s.ephemeralStore.AddLog(log)
}

// Logs returns the logs of current transaction.
func (s *StateDB) Logs() []*ethtypes.Log {
	return s.ephemeralStore.GetLogs()
}

// AddRefund adds gas to the refund counter
func (s *StateDB) AddRefund(gas uint64) {
	s.ephemeralStore.AddRefund(gas)
}

// SubRefund removes gas from the refund counter.
// This method will panic if the refund counter goes below zero
func (s *StateDB) SubRefund(gas uint64) {
	s.ephemeralStore.SubRefund(gas)
}

// Exist reports whether the given account address exists in the state.
// Notably this also returns true for suicided accounts.
func (s *StateDB) Exist(addr common.Address) bool {
	account := s.keeper.GetAccount(s.ctx.CurrentCtx(), addr)
	return account != nil
}

// Empty returns whether the state object is either non-existent
// or empty according to the EIP161 specification (balance = nonce = code = 0)
func (s *StateDB) Empty(addr common.Address) bool {
	account := s.keeper.GetAccount(s.ctx.CurrentCtx(), addr)
	if account == nil {
		return true
	}

	return account.Balance.Sign() == 0 && account.Nonce == 0 && bytes.Equal(account.CodeHash, emptyCodeHash)
}

// GetBalance retrieves the balance from the given address or 0 if object not found
func (s *StateDB) GetBalance(addr common.Address) *big.Int {
	account := s.keeper.GetAccount(s.ctx.CurrentCtx(), addr)
	if account != nil {
		return account.Balance
	}

	return common.Big0
}

// GetNonce returns the nonce of account, 0 if not exists.
func (s *StateDB) GetNonce(addr common.Address) uint64 {
	account := s.keeper.GetAccount(s.ctx.CurrentCtx(), addr)
	if account != nil {
		return account.Nonce
	}

	return 0
}

// GetCode returns the code of account, nil if not exists.
func (s *StateDB) GetCode(addr common.Address) []byte {
	account := s.keeper.GetAccount(s.ctx.CurrentCtx(), addr)
	if account == nil {
		return nil
	}

	if bytes.Equal(account.CodeHash, emptyCodeHash) {
		return nil
	}

	return s.keeper.GetCode(s.ctx.CurrentCtx(), common.BytesToHash(account.CodeHash))
}

// GetCodeSize returns the code size of account.
func (s *StateDB) GetCodeSize(addr common.Address) int {
	code := s.GetCode(addr)
	return len(code)
}

// GetCodeHash returns the code hash of account.
func (s *StateDB) GetCodeHash(addr common.Address) common.Hash {
	account := s.keeper.GetAccount(s.ctx.CurrentCtx(), addr)
	if account == nil {
		return common.Hash{}
	}

	return common.BytesToHash(account.CodeHash)
}

// GetState retrieves a value from the given account's storage trie.
func (s *StateDB) GetState(addr common.Address, hash common.Hash) common.Hash {
	account := s.getOrNewAccount(addr)
	if account == nil {
		return common.Hash{}
	}

	return s.keeper.GetState(s.ctx.CurrentCtx(), addr, hash)
}

// GetCommittedState retrieves a value from the given account's committed storage trie.
func (s *StateDB) GetCommittedState(addr common.Address, hash common.Hash) common.Hash {
	// This gets the state from the parent ctx which is the state before Commit()
	return s.keeper.GetState(s.ctx.initialCtx, addr, hash)
}

// GetRefund returns the current value of the refund counter.
func (s *StateDB) GetRefund() uint64 {
	return s.ephemeralStore.GetRefund()
}

// HasSuicided returns if the contract is suicided in current transaction.
func (s *StateDB) HasSuicided(addr common.Address) bool {
	return s.ephemeralStore.GetAccountSuicided(addr)
}

// AddPreimage records a SHA3 preimage seen by the VM.
// AddPreimage performs a no-op since the EnablePreimageRecording flag is disabled
// on the vm.Config during state transitions. No store trie preimages are written
// to the database.
func (s *StateDB) AddPreimage(hash common.Hash, preimage []byte) {} //nolint: revive

// getOrNewAccount retrieves a state account or create a new account if nil.
func (s *StateDB) getOrNewAccount(addr common.Address) *Account {
	account := s.keeper.GetAccount(s.ctx.CurrentCtx(), addr)
	if account == nil {
		account = NewEmptyAccount()
	}

	return account
}

// CreateAccount explicitly creates a state object. If a state object with the address
// already exists the balance is carried over to the new account.
//
// CreateAccount is called during the EVM CREATE operation. The situation might arise that
// a contract does the following:
//
// 1. sends funds to sha(account ++ (nonce + 1))
// 2. tx_create(sha(account ++ nonce)) (note that this gets the address of 1)
//
// Carrying over the balance ensures that Ether doesn't disappear.
func (s *StateDB) CreateAccount(addr common.Address) {
	account := s.keeper.GetAccount(s.ctx.CurrentCtx(), addr)
	if account == nil {
		// No account found, create a new one
		if err := s.keeper.SetAccount(s.ctx.CurrentCtx(), addr, *NewEmptyAccount()); err != nil {
			s.SetError(fmt.Errorf("failed to create account: %w", err))
		}

		return
	}

	// If there is already an account, zero out everything except for the balance ?
	// This is done in previous StateDB

	// Create a new account -- Must use NewEmptyAccount() so that the
	// CodeHash is the actual hash of nil, not an empty byte slice
	newAccount := NewEmptyAccount()
	newAccount.Balance = account.Balance

	if err := s.keeper.SetAccount(s.ctx.CurrentCtx(), addr, *newAccount); err != nil {
		s.SetError(fmt.Errorf("failed to create account: %w", err))
	}
}

// ForEachStorage iterate the contract storage, the iteration order is not defined.
func (s *StateDB) ForEachStorage(addr common.Address, cb func(key, value common.Hash) bool) error {
	s.keeper.ForEachStorage(s.ctx.initialCtx, addr, cb)
	return nil
}

/*
 * SETTERS
 */

// AddBalance adds amount to the account associated with addr.
func (s *StateDB) AddBalance(addr common.Address, amount *big.Int) {
	// Geth apparently allows negative amounts, but can cause negative
	// balance which is not allowed in bank keeper. However, we need to create
	// the account still.

	account := s.getOrNewAccount(addr)

	account.Balance = new(big.Int).Add(account.Balance, amount)
	if err := s.keeper.SetAccount(s.ctx.CurrentCtx(), addr, *account); err != nil {
		s.SetError(fmt.Errorf("failed to set account for balance addition: %w", err))
	}
}

// SubBalance subtracts amount from the account associated with addr.
func (s *StateDB) SubBalance(addr common.Address, amount *big.Int) {
	// Avoid returning on 0 value to allow for account to be created still.
	// This should be a non-issue if state clearing is implemented, as if there
	// is an non-existent account and 0 balance is added, an account isn't
	// created.
	account := s.getOrNewAccount(addr)

	account.Balance = new(big.Int).Sub(account.Balance, amount)
	if err := s.keeper.SetAccount(s.ctx.CurrentCtx(), addr, *account); err != nil {
		s.SetError(fmt.Errorf("failed to set account for balance subtraction: %w", err))
	}
}

// SetNonce sets the nonce of account.
func (s *StateDB) SetNonce(addr common.Address, nonce uint64) {
	account := s.getOrNewAccount(addr)

	account.Nonce = nonce
	if err := s.keeper.SetAccount(s.ctx.CurrentCtx(), addr, *account); err != nil {
		s.SetError(fmt.Errorf("failed to set account for nonce: %w", err))
	}
}

// SetCode sets the code of account.
func (s *StateDB) SetCode(addr common.Address, code []byte) {
	account := s.getOrNewAccount(addr)
	account.CodeHash = crypto.Keccak256Hash(code).Bytes()

	// Set account so CodeHash is updated
	if err := s.keeper.SetAccount(s.ctx.CurrentCtx(), addr, *account); err != nil {
		s.SetError(fmt.Errorf("failed to set account for code: %w", err))
	}

	s.keeper.SetCode(s.ctx.CurrentCtx(), account.CodeHash, code)
}

// SetState sets the contract state.
func (s *StateDB) SetState(addr common.Address, key, value common.Hash) {
	acc := s.getOrNewAccount(addr)
	if err := s.keeper.SetAccount(s.ctx.CurrentCtx(), addr, *acc); err != nil {
		s.SetError(fmt.Errorf("failed to set account for state: %w", err))
	}

	// We cannot attempt to skip noop changes by just checking committed state
	// Example:
	// 1. With committed state to 0x0
	// 2. Dirty change to 0x1
	// 3. Dirty change to 0x0 - cannot skip this
	// 4. Commit
	//
	// End result: 0x0, but we cannot skip step 3 or it will be incorrectly 0x1
	s.keeper.SetState(s.ctx.CurrentCtx(), addr, key, value)

	// Keep track of the key that had a state change
	s.ephemeralStore.AddContractStateKey(addr, key)
}

// Suicide marks the given account as suicided.
// This clears the account balance.
//
// The account's state object is still available until the state is committed,
// getStateObject will return a non-nil account after Suicide.
func (s *StateDB) Suicide(addr common.Address) bool {
	account := s.keeper.GetAccount(s.ctx.CurrentCtx(), addr)
	if account == nil {
		return false
	}

	// Balance cleared, but code and state should still be available until Commit()
	if err := s.keeper.SetBalance(s.ctx.CurrentCtx(), addr, common.Big0); err != nil {
		s.SetError(fmt.Errorf("failed to remove suicide account balance: %w", err))
	}

	s.ephemeralStore.SetAccountSuicided(addr)

	return true
}

// PrepareAccessList handles the preparatory steps for executing a state transition with
// regards to both EIP-2929 and EIP-2930:
//
// - Add sender to access list (2929)
// - Add destination to access list (2929)
// - Add precompiles to access list (2929)
// - Add the contents of the optional tx access list (2930)
//
// This method should only be called if Yolov3/Berlin/2929+2930 is applicable at the current number.
func (s *StateDB) PrepareAccessList(sender common.Address, dst *common.Address, precompiles []common.Address, list ethtypes.AccessList) {
	s.AddAddressToAccessList(sender)
	if dst != nil {
		s.AddAddressToAccessList(*dst)
		// If it's a create-tx, the destination will be added inside evm.create
	}
	for _, addr := range precompiles {
		s.AddAddressToAccessList(addr)
	}
	for _, el := range list {
		s.AddAddressToAccessList(el.Address)
		for _, key := range el.StorageKeys {
			s.AddSlotToAccessList(el.Address, key)
		}
	}
}

// AddAddressToAccessList adds the given address to the access list
func (s *StateDB) AddAddressToAccessList(addr common.Address) {
	if s.accessList.AddAddress(addr) {
		s.journal.append(accessListAddAccountChange{&addr})
	}
}

// AddSlotToAccessList adds the given (address, slot)-tuple to the access list
func (s *StateDB) AddSlotToAccessList(addr common.Address, slot common.Hash) {
	addrMod, slotMod := s.accessList.AddSlot(addr, slot)
	if addrMod {
		// In practice, this should not happen, since there is no way to enter the
		// scope of 'address' without having the 'address' become already added
		// to the access list (via call-variant, create, etc).
		// Better safe than sorry, though
		s.journal.append(accessListAddAccountChange{&addr})
	}
	if slotMod {
		s.journal.append(accessListAddSlotChange{
			address: &addr,
			slot:    &slot,
		})
	}
}

// AddressInAccessList returns true if the given address is in the access list.
func (s *StateDB) AddressInAccessList(addr common.Address) bool {
	return s.accessList.ContainsAddress(addr)
}

// SlotInAccessList returns true if the given (address, slot)-tuple is in the access list.
func (s *StateDB) SlotInAccessList(addr common.Address, slot common.Hash) (addressPresent bool, slotPresent bool) {
	return s.accessList.Contains(addr, slot)
}

// Snapshot returns an identifier for the current revision of the state.
func (s *StateDB) Snapshot() int {
	return s.ctx.Snapshot(
		s.journal.length(),
		s.ephemeralStore.GetRevertKey(),
	)
}

// RevertToSnapshot reverts all state changes made since the given revision.
func (s *StateDB) RevertToSnapshot(revid int) {
	s.ctx.Revert(revid)

	currentSnapshot, found := s.ctx.CurrentSnapshot()
	if !found {
		panic(fmt.Errorf("current snapshot with id %d not found", revid))
	}

	// Revert journal to the latest snapshot's journal index
	s.journal.Revert(s, currentSnapshot.journalIndex)

	// Revert ephemeral store: refunds, logs, suicided accounts
	s.ephemeralStore.Revert(currentSnapshot.storeRevertKey)
}

// the StateDB object should be discarded after committed.
func (s *StateDB) Commit() error {
	// If there was an error during execution, commit will return error without
	// persisting any changes to the underlying ctx. Instead of panicking at the
	// call, we return the error here so it is more visible.
	if s.sdkError != nil {
		return s.sdkError
	}

	// Delete suicided accounts -- these still need to be committed
	suicidedAddrs := s.ephemeralStore.GetAllSuicided()
	for _, addr := range suicidedAddrs {
		// Balance is also cleared as part of Keeper.DeleteAccount
		if err := s.keeper.DeleteAccount(s.ctx.CurrentCtx(), addr); err != nil {
			return fmt.Errorf("failed to delete suicided account: %w", err)
		}
	}

	// Check for any state no-op changes and skip committing
	for _, change := range s.ephemeralStore.GetContractStateKeys() {
		committedValue := s.keeper.GetState(s.ctx.initialCtx, change.Addr, change.Key)
		dirtyValue := s.keeper.GetState(s.ctx.CurrentCtx(), change.Addr, change.Key)

		// Different committed and dirty value, keep it for commit
		if committedValue != dirtyValue {
			continue
		}

		// Remove no-op change from ALL snapshot contexts to prevent writing it
		// to the store. This is necessary since the same key/value will still
		// update the internal IAVL node version and thus the hash.

		// We can't just remove it from the latest snapshot context, since all
		// snapshots are written to the store. A snapshot in the "middle" may
		// contain a state change that isn't just a no-op change but a
		// completely different value.
		// Example: A -(snapshot)-> B -(snapshot)-> A
		// -> We need to remove BOTH the B -> A change and the A -> B change,
		//    otherwise B will be written to the store.
		for _, snapshot := range s.ctx.snapshots {
			if err := s.keeper.UnsetState(snapshot.ctx, change.Addr, change.Key); err != nil {
				return err
			}
		}
	}

	// Commit after account deletions
	s.ctx.Commit()

	// Journal only contains non-state content, so nothing to commit.
	return nil
}

// SetError sets the error in the StateDB which will be returned on Commit. This
// only sets the first error that occurs. Subsequent calls to SetError will be
// ignored as the initial error is the most important. Any errors that occur
// after the first error may be due to invalid state caused by the first error.
func (s *StateDB) SetError(err error) {
	if s.sdkError != nil {
		return
	}

	s.sdkError = err
}
