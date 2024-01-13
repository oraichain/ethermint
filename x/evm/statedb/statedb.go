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
	"sort"

	"github.com/cosmos/cosmos-sdk/store"
	storetypes "github.com/cosmos/cosmos-sdk/store/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/ethereum/go-ethereum/common"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/core/vm"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/evmos/ethermint/x/evm/types"
	evm "github.com/evmos/ethermint/x/evm/vm"
	dbm "github.com/tendermint/tm-db"
)

// revision is the identifier of a version of state.
// it consists of an auto-increment id and a journal index.
// it's safer to use than using journal index alone.
type revision struct {
	id           int
	journalIndex int
}

type ctxSnapshot struct {
	id    int
	ctx   sdk.Context
	write func()
}

var _ vm.StateDB = &StateDB{}

// StateDB structs within the ethereum protocol are used to store anything
// within the merkle trie. StateDBs take care of caching and storing
// nested states. It's the general query interface to retrieve:
// * Contracts
// * Accounts
type StateDB struct {
	keeper evm.StateDBKeeper

	// current branched sdk.Context -- branched on creation and each snapshot
	currentCtx sdk.Context
	// write function of currentCtx
	currentCtxWrite func()

	// Snapshots of Ctx, each a CacheContext branch of the previous one
	ctxSnapshots   []ctxSnapshot
	nextSnapshotID int

	logStore *StateDBStore

	// Journal of state modifications. This is the backbone of
	// Snapshot and RevertToSnapshot.
	journal *journal
	// validRevisions []revision
	// nextRevisionID int

	// stateObjects map[common.Address]*stateObject

	txConfig types.TxConfig

	// The refund counter, also used by state transitioning.
	refund uint64

	// Per-transaction logs
	logs []*ethtypes.Log

	// Per-transaction access list
	accessList *accessList
}

// New creates a new state from a given trie.
func New(ctx sdk.Context, keeper evm.StateDBKeeper, txConfig types.TxConfig) evm.StateDB {
	// Create an in-memory DB for accessList which does not need to be committed
	// to underlying state.
	//
	// Why do we use this instead of a simple map?
	// Because Snapshot() will then also apply to the accessList without needing
	// to keep a separate snapshot of the accessList.
	db := dbm.NewMemDB()
	cms := store.NewCommitMultiStore(db)
	cms.MountStoreWithDB(storeKey, storetypes.StoreTypeMemory, db)

	statedb := &StateDB{
		keeper:          keeper,
		currentCtx:      ctx,
		currentCtxWrite: nil,

		ctxSnapshots:   []ctxSnapshot{},
		nextSnapshotID: 0,

		journal:    newJournal(),
		accessList: newAccessList(),
		logStore:   NewStateDBStore(storeKey),

		txConfig: txConfig,
	}

	// Create an initial cache ctx branch so we don't modify parent Context
	// without calling Commit()
	_ = statedb.Snapshot()

	return statedb
}

// Keeper returns the underlying `Keeper`
func (s *StateDB) Keeper() evm.StateDBKeeper {
	return s.keeper
}

// AddLog adds a log, called by evm.
func (s *StateDB) AddLog(log *ethtypes.Log) {
	log.TxHash = s.txConfig.TxHash
	log.BlockHash = s.txConfig.BlockHash
	log.TxIndex = s.txConfig.TxIndex
	log.Index = s.txConfig.LogIndex + uint(len(s.logs))

	s.logStore.AddLog(s.currentCtx, log)
}

// Logs returns the logs of current transaction.
func (s *StateDB) Logs() []*ethtypes.Log {
	return s.logStore.GetAllLogs(s.currentCtx)
}

// AddRefund adds gas to the refund counter
func (s *StateDB) AddRefund(gas uint64) {
	s.logStore.AddRefund(s.currentCtx, gas)
}

// SubRefund removes gas from the refund counter.
// This method will panic if the refund counter goes below zero
func (s *StateDB) SubRefund(gas uint64) {
	s.logStore.SubRefund(s.currentCtx, gas)
}

// Exist reports whether the given account address exists in the state.
// Notably this also returns true for suicided accounts.
func (s *StateDB) Exist(addr common.Address) bool {
	account := s.keeper.GetAccount(s.currentCtx, addr)
	return account != nil
}

// Empty returns whether the state object is either non-existent
// or empty according to the EIP161 specification (balance = nonce = code = 0)
func (s *StateDB) Empty(addr common.Address) bool {
	account := s.keeper.GetAccount(s.currentCtx, addr)
	if account == nil {
		return true
	}

	return account.Balance.Sign() == 0 && account.Nonce == 0 && bytes.Equal(account.CodeHash, emptyCodeHash)
}

// GetBalance retrieves the balance from the given address or 0 if object not found
func (s *StateDB) GetBalance(addr common.Address) *big.Int {
	account := s.keeper.GetAccount(s.currentCtx, addr)
	if account != nil {
		return account.Balance
	}

	return common.Big0
}

// GetNonce returns the nonce of account, 0 if not exists.
func (s *StateDB) GetNonce(addr common.Address) uint64 {
	account := s.keeper.GetAccount(s.currentCtx, addr)
	if account != nil {
		return account.Nonce
	}

	return 0
}

// GetCode returns the code of account, nil if not exists.
func (s *StateDB) GetCode(addr common.Address) []byte {
	account := s.keeper.GetAccount(s.currentCtx, addr)
	if account == nil {
		return nil
	}

	if bytes.Equal(account.CodeHash, emptyCodeHash) {
		return nil
	}

	return s.keeper.GetCode(s.currentCtx, common.BytesToHash(account.CodeHash))
}

// GetCodeSize returns the code size of account.
func (s *StateDB) GetCodeSize(addr common.Address) int {
	code := s.GetCode(addr)
	return len(code)
}

// GetCodeHash returns the code hash of account.
func (s *StateDB) GetCodeHash(addr common.Address) common.Hash {
	account := s.keeper.GetAccount(s.currentCtx, addr)
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

	return s.keeper.GetState(s.currentCtx, addr, hash)
}

// GetCommittedState retrieves a value from the given account's committed storage trie.
func (s *StateDB) GetCommittedState(addr common.Address, hash common.Hash) common.Hash {
	// TODO: Double check or find a cleaner way
	// This gets the state from the parent ctx which is the state before Commit()
	return s.keeper.GetState(s.ctxSnapshots[0].ctx, addr, hash)
}

// GetRefund returns the current value of the refund counter.
func (s *StateDB) GetRefund() uint64 {
	return s.logStore.GetRefund(s.currentCtx)
}

// HasSuicided returns if the contract is suicided in current transaction.
func (s *StateDB) HasSuicided(addr common.Address) bool {
	// Could be created and then suicided in the same transaction, so we can't
	// rely on the existence of the account before the current transaction.

	stateObject := s.getStateObject(addr)
	if stateObject != nil {
		return stateObject.suicided
	}
	return false
}

// AddPreimage records a SHA3 preimage seen by the VM.
// AddPreimage performs a no-op since the EnablePreimageRecording flag is disabled
// on the vm.Config during state transitions. No store trie preimages are written
// to the database.
func (s *StateDB) AddPreimage(hash common.Hash, preimage []byte) {} //nolint: revive

// getOrNewAccount retrieves a state account or create a new account if nil.
func (s *StateDB) getOrNewAccount(addr common.Address) *types.StateDBAccount {
	account := s.keeper.GetAccount(s.currentCtx, addr)
	if account == nil {
		account = &types.StateDBAccount{}
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
	account := s.keeper.GetAccount(s.currentCtx, addr)

	if account != nil {
		// Only carry over balance, other values are zero'd out ?
		account.Balance = account.Balance
		s.keeper.SetAccount(s.currentCtx, addr, *account)
	}
}

// ForEachStorage iterate the contract storage, the iteration order is not defined.
func (s *StateDB) ForEachStorage(addr common.Address, cb func(key, value common.Hash) bool) error {
	s.keeper.ForEachStorage(s.currentCtx, addr, cb)
	return nil
}

/*
 * SETTERS
 */

// AddBalance adds amount to the account associated with addr.
func (s *StateDB) AddBalance(addr common.Address, amount *big.Int) {
	if amount.Sign() == 0 {
		return
	}

	account := s.getOrNewAccount(addr)

	account.Balance = new(big.Int).Add(account.Balance, amount)
	s.keeper.SetAccount(s.currentCtx, addr, *account)
}

// SubBalance subtracts amount from the account associated with addr.
func (s *StateDB) SubBalance(addr common.Address, amount *big.Int) {
	if amount.Sign() == 0 {
		return
	}

	account := s.getOrNewAccount(addr)

	account.Balance = new(big.Int).Sub(account.Balance, amount)
	s.keeper.SetAccount(s.currentCtx, addr, *account)
}

// SetNonce sets the nonce of account.
func (s *StateDB) SetNonce(addr common.Address, nonce uint64) {
	account := s.getOrNewAccount(addr)

	account.Nonce = nonce
	s.keeper.SetAccount(s.currentCtx, addr, *account)
}

// SetCode sets the code of account.
func (s *StateDB) SetCode(addr common.Address, code []byte) {
	s.keeper.SetCode(s.currentCtx, crypto.Keccak256Hash(code).Bytes(), code)
}

// SetState sets the contract state.
func (s *StateDB) SetState(addr common.Address, key, value common.Hash) {
	s.keeper.SetState(s.currentCtx, addr, key, value.Bytes())
}

// Suicide marks the given account as suicided.
// This clears the account balance.
//
// The account's state object is still available until the state is committed,
// getStateObject will return a non-nil account after Suicide.
func (s *StateDB) Suicide(addr common.Address) bool {
	account := s.keeper.GetAccount(s.currentCtx, addr)

	if account == nil {
		return false
	}

	if err := s.keeper.DeleteAccount(s.currentCtx, addr); err != nil {
		panic(fmt.Errorf("failed to delete account in Suicide(): %w", err))
	}

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
	id := s.nextSnapshotID
	s.nextSnapshotID++

	// Save the current context (cached multi-store and events) + write function
	// to apply the snapshot to the parent store
	s.ctxSnapshots = append(s.ctxSnapshots, ctxSnapshot{
		id:    id,
		ctx:   s.currentCtx,
		write: s.currentCtxWrite,
	})

	// Branch off a new CacheMultiStore + write function
	newCtx, write := s.currentCtx.CacheContext()

	// Update ctx to the new branch
	s.currentCtx = newCtx
	s.currentCtxWrite = write

	return id
}

// RevertToSnapshot reverts all state changes made since the given revision.
func (s *StateDB) RevertToSnapshot(revid int) {
	// Find the snapshot in the stack of valid snapshots.
	idx := sort.Search(len(s.ctxSnapshots), func(i int) bool {
		return s.ctxSnapshots[i].id >= revid
	})

	if idx == len(s.ctxSnapshots) || s.ctxSnapshots[idx].id != revid {
		panic(fmt.Errorf("revision id %v cannot be reverted", revid))
	}

	// Update current ctx to the snapshot ctx
	s.currentCtx = s.ctxSnapshots[idx].ctx

	// Remove invalidated snapshots
	s.ctxSnapshots = s.ctxSnapshots[:idx]
}

// the StateDB object should be discarded after committed.
func (s *StateDB) Commit() error {
	// Write snapshots from newest to oldest.
	// Each store.Write() applies the state changes to its parent / previous snapshot
	for i := len(s.ctxSnapshots) - 1; i >= 0; i-- {
		snapshot := s.ctxSnapshots[i]

		// write() will be nil for the root snapshot that was created on New()
		// Root ctx won't need write() since it isn't a CacheContext
		if snapshot.write != nil {
			snapshot.write()
		}
	}

	return nil
}
