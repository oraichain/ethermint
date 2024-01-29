package statedb

import (
	"fmt"

	"github.com/ethereum/go-ethereum/common"
)

// StoreRevertKey defines the required information to revert to a previous state.
type StoreRevertKey struct {
	RefundIndex           int
	SuicidedAccountsIndex int
}

// EphemeralStore provides in-memory state of the refund and suicided accounts
// state with the ability to revert to a previous state.
type EphemeralStore struct {
	refundStates          []uint64
	suicidedAccountStates []common.Address
}

// NewStateDBStore creates a new EphemeralStore.
func NewStateDBStore() *EphemeralStore {
	return &EphemeralStore{
		refundStates:          []uint64{},
		suicidedAccountStates: []common.Address{},
	}
}

// GetRevertKey returns the identifier of the current state of the store.
func (ls *EphemeralStore) GetRevertKey() StoreRevertKey {
	return StoreRevertKey{
		RefundIndex:           len(ls.refundStates),
		SuicidedAccountsIndex: len(ls.suicidedAccountStates),
	}
}

// Revert reverts the state to the given key.
func (ls *EphemeralStore) Revert(key StoreRevertKey) {
	if key.RefundIndex > len(ls.refundStates) {
		panic(fmt.Errorf(
			"invalid RefundIndex, %d is greater than the length of the refund states (%d)",
			key.RefundIndex, len(ls.refundStates),
		))
	}

	if key.SuicidedAccountsIndex > len(ls.suicidedAccountStates) {
		panic(fmt.Errorf(
			"invalid SuicidedAccountsIndex, %d is greater than the length of the suicided accounts (%d)",
			key.SuicidedAccountsIndex, len(ls.suicidedAccountStates),
		))
	}

	ls.refundStates = ls.refundStates[:key.RefundIndex]
	ls.suicidedAccountStates = ls.suicidedAccountStates[:key.SuicidedAccountsIndex]
}

// -----------------------------------------------------------------------------
// Refund

// GetRefund returns the current refund value, which is the last element.
func (ls *EphemeralStore) GetRefund() uint64 {
	if len(ls.refundStates) == 0 {
		return 0
	}

	return ls.refundStates[len(ls.refundStates)-1]
}

// AddRefund adds a refund to the store.
func (ls *EphemeralStore) AddRefund(gas uint64) {
	newRefund := ls.GetRefund() + gas
	ls.refundStates = append(ls.refundStates, newRefund)
}

// SubRefund subtracts a refund from the store.
func (ls *EphemeralStore) SubRefund(gas uint64) {
	currentRefund := ls.GetRefund()

	if currentRefund < gas {
		panic("current refund is less than gas")
	}

	newRefund := currentRefund - gas
	ls.refundStates = append(ls.refundStates, newRefund)
}

// -----------------------------------------------------------------------------
// Suicided accounts

// SetAccountSuicided sets the given account as suicided.
func (ls *EphemeralStore) SetAccountSuicided(addr common.Address) {
	// If already in the list, ignore
	if ls.GetAccountSuicided(addr) {
		return
	}

	ls.suicidedAccountStates = append(ls.suicidedAccountStates, addr)
}

// GetAccountSuicided returns true if the given account is suicided.
func (ls *EphemeralStore) GetAccountSuicided(addr common.Address) bool {
	for _, a := range ls.suicidedAccountStates {
		if a == addr {
			return true
		}
	}

	return false
}

// GetAllSuicided returns all suicided accounts.
func (ls *EphemeralStore) GetAllSuicided() []common.Address {
	return ls.suicidedAccountStates
}
