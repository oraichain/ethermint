package ctxstatedb

import (
	"fmt"

	"github.com/ethereum/go-ethereum/common"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
)

// StoreRevertKey defines the required information to revert to a previous state.
type StoreRevertKey struct {
	RefundIndex           int
	SuicidedAccountsIndex int
	LogsIndex             int
	ContractStatesIndex   int
	CreatedAccountsIndex  int
}

// ContractStateKey represents the state key of a contract.
type ContractStateKey struct {
	Addr common.Address
	Key  common.Hash
}

// EphemeralStore provides in-memory state of the refund and suicided accounts
// state with the ability to revert to a previous state.
type EphemeralStore struct {
	RefundStates          []uint64
	SuicidedAccountStates []common.Address
	Logs                  []*ethtypes.Log
	ContractStates        []ContractStateKey
	CreatedAccounts       []common.Address
}

// NewEphemeralStore creates a new EphemeralStore.
func NewEphemeralStore() *EphemeralStore {
	return &EphemeralStore{
		RefundStates:          nil,
		SuicidedAccountStates: nil,
		Logs:                  nil,
		ContractStates:        nil,
		CreatedAccounts:       nil,
	}
}

// GetRevertKey returns the identifier of the current state of the store.
func (es *EphemeralStore) GetRevertKey() StoreRevertKey {
	return StoreRevertKey{
		RefundIndex:           len(es.RefundStates),
		SuicidedAccountsIndex: len(es.SuicidedAccountStates),
		LogsIndex:             len(es.Logs),
		ContractStatesIndex:   len(es.ContractStates),
		CreatedAccountsIndex:  len(es.CreatedAccounts),
	}
}

// Revert reverts the state to the given key.
func (es *EphemeralStore) Revert(key StoreRevertKey) {
	if err := es.ValidateRevertKey(key); err != nil {
		panic(err)
	}

	es.RefundStates = es.RefundStates[:key.RefundIndex]
	es.SuicidedAccountStates = es.SuicidedAccountStates[:key.SuicidedAccountsIndex]
	es.Logs = es.Logs[:key.LogsIndex]
	es.ContractStates = es.ContractStates[:key.ContractStatesIndex]
	es.CreatedAccounts = es.CreatedAccounts[:key.CreatedAccountsIndex]
}

func (es *EphemeralStore) ValidateRevertKey(key StoreRevertKey) error {
	if key.RefundIndex > len(es.RefundStates) {
		return fmt.Errorf(
			"invalid RefundIndex, %d is greater than the length of the refund states (%d)",
			key.RefundIndex, len(es.RefundStates),
		)
	}

	if key.SuicidedAccountsIndex > len(es.SuicidedAccountStates) {
		return fmt.Errorf(
			"invalid SuicidedAccountsIndex, %d is greater than the length of the suicided accounts (%d)",
			key.SuicidedAccountsIndex, len(es.SuicidedAccountStates),
		)
	}

	if key.LogsIndex > len(es.Logs) {
		return fmt.Errorf(
			"invalid LogsIndex, %d is greater than the length of the logs (%d)",
			key.LogsIndex, len(es.Logs),
		)
	}

	if key.ContractStatesIndex > len(es.ContractStates) {
		return fmt.Errorf(
			"invalid ContractStatesIndex, %d is greater than the length of the contract states (%d)",
			key.ContractStatesIndex, len(es.ContractStates),
		)
	}

	if key.CreatedAccountsIndex > len(es.CreatedAccounts) {
		return fmt.Errorf(
			"invalid CreatedAccountsIndex, %d is greater than the length of the created accounts (%d)",
			key.CreatedAccountsIndex, len(es.CreatedAccounts),
		)
	}

	return nil
}

// -----------------------------------------------------------------------------
// Logs

// AddLog adds a log to the store.
func (es *EphemeralStore) AddLog(log *ethtypes.Log) {
	es.Logs = append(es.Logs, log)
}

// GetLogs returns all logs.
func (es *EphemeralStore) GetLogs() []*ethtypes.Log {
	return es.Logs
}

// -----------------------------------------------------------------------------
// Refund

// GetRefund returns the current refund value, which is the last element.
func (es *EphemeralStore) GetRefund() uint64 {
	if len(es.RefundStates) == 0 {
		return 0
	}

	return es.RefundStates[len(es.RefundStates)-1]
}

// AddRefund adds a refund to the store.
func (es *EphemeralStore) AddRefund(gas uint64) {
	newRefund := es.GetRefund() + gas
	es.RefundStates = append(es.RefundStates, newRefund)
}

// SubRefund subtracts a refund from the store.
func (es *EphemeralStore) SubRefund(gas uint64) {
	currentRefund := es.GetRefund()

	if currentRefund < gas {
		panic("current refund is less than gas")
	}

	newRefund := currentRefund - gas
	es.RefundStates = append(es.RefundStates, newRefund)
}

// -----------------------------------------------------------------------------
// Suicided accounts

// SetAccountSuicided sets the given account as suicided.
func (es *EphemeralStore) SetAccountSuicided(addr common.Address) {
	// If already in the list, ignore
	if es.GetAccountSuicided(addr) {
		return
	}

	es.SuicidedAccountStates = append(es.SuicidedAccountStates, addr)
}

// GetAccountSuicided returns true if the given account is suicided.
func (es *EphemeralStore) GetAccountSuicided(addr common.Address) bool {
	for _, a := range es.SuicidedAccountStates {
		if a == addr {
			return true
		}
	}

	return false
}

// GetAllSuicided returns all suicided accounts.
func (es *EphemeralStore) GetAllSuicided() []common.Address {
	return es.SuicidedAccountStates
}

// -----------------------------------------------------------------------------
// Contract states

// AddContractState adds a contract state to the store.
func (es *EphemeralStore) AddContractStateKey(addr common.Address, key common.Hash) {
	if es.HasContractStateKey(addr, key) {
		return
	}

	es.ContractStates = append(es.ContractStates, ContractStateKey{Addr: addr, Key: key})
}

func (es *EphemeralStore) HasContractStateKey(addr common.Address, key common.Hash) bool {
	for _, state := range es.ContractStates {
		if state.Addr == addr && state.Key == key {
			return true
		}
	}

	return false
}

// GetContractStateKeys returns all contract state keys.
func (es *EphemeralStore) GetContractStateKeys() []ContractStateKey {
	return es.ContractStates
}

// -----------------------------------------------------------------------------
// Created accounts

// AddCreatedAccount adds a created account to the store.
func (es *EphemeralStore) AddCreatedAccount(addr common.Address) {
	if es.HasCreatedAccount(addr) {
		return
	}

	es.CreatedAccounts = append(es.CreatedAccounts, addr)
}

func (es *EphemeralStore) HasCreatedAccount(addr common.Address) bool {
	for _, a := range es.CreatedAccounts {
		if a == addr {
			return true
		}
	}

	return false
}

// GetAllCreatedAccounts returns all created accounts.
func (es *EphemeralStore) GetAllCreatedAccounts() []common.Address {
	return es.CreatedAccounts
}
