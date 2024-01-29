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

// NewEphemeralStore creates a new EphemeralStore.
func NewEphemeralStore() *EphemeralStore {
	return &EphemeralStore{
		refundStates:          []uint64{},
		suicidedAccountStates: []common.Address{},
	}
}

// GetRevertKey returns the identifier of the current state of the store.
func (es *EphemeralStore) GetRevertKey() StoreRevertKey {
	return StoreRevertKey{
		RefundIndex:           len(es.refundStates),
		SuicidedAccountsIndex: len(es.suicidedAccountStates),
	}
}

// Revert reverts the state to the given key.
func (es *EphemeralStore) Revert(key StoreRevertKey) {
	if key.RefundIndex > len(es.refundStates) {
		panic(fmt.Errorf(
			"invalid RefundIndex, %d is greater than the length of the refund states (%d)",
			key.RefundIndex, len(es.refundStates),
		))
	}

	if key.SuicidedAccountsIndex > len(es.suicidedAccountStates) {
		panic(fmt.Errorf(
			"invalid SuicidedAccountsIndex, %d is greater than the length of the suicided accounts (%d)",
			key.SuicidedAccountsIndex, len(es.suicidedAccountStates),
		))
	}

	es.refundStates = es.refundStates[:key.RefundIndex]
	es.suicidedAccountStates = es.suicidedAccountStates[:key.SuicidedAccountsIndex]
}

// -----------------------------------------------------------------------------
// Refund

// GetRefund returns the current refund value, which is the last element.
func (es *EphemeralStore) GetRefund() uint64 {
	if len(es.refundStates) == 0 {
		return 0
	}

	return es.refundStates[len(es.refundStates)-1]
}

// AddRefund adds a refund to the store.
func (es *EphemeralStore) AddRefund(gas uint64) {
	newRefund := es.GetRefund() + gas
	es.refundStates = append(es.refundStates, newRefund)
}

// SubRefund subtracts a refund from the store.
func (es *EphemeralStore) SubRefund(gas uint64) {
	currentRefund := es.GetRefund()

	if currentRefund < gas {
		panic("current refund is less than gas")
	}

	newRefund := currentRefund - gas
	es.refundStates = append(es.refundStates, newRefund)
}

// -----------------------------------------------------------------------------
// Suicided accounts

// SetAccountSuicided sets the given account as suicided.
func (es *EphemeralStore) SetAccountSuicided(addr common.Address) {
	// If already in the list, ignore
	if es.GetAccountSuicided(addr) {
		return
	}

	es.suicidedAccountStates = append(es.suicidedAccountStates, addr)
}

// GetAccountSuicided returns true if the given account is suicided.
func (es *EphemeralStore) GetAccountSuicided(addr common.Address) bool {
	for _, a := range es.suicidedAccountStates {
		if a == addr {
			return true
		}
	}

	return false
}

// GetAllSuicided returns all suicided accounts.
func (es *EphemeralStore) GetAllSuicided() []common.Address {
	return es.suicidedAccountStates
}
