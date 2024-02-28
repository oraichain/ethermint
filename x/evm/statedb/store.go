package statedb

import (
	"fmt"

	"github.com/ethereum/go-ethereum/common"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
)

// initialStoreRevertKey is the initial state of the store.
var initialStoreRevertKey = StoreRevertKey{
	RefundIndex:           0,
	SuicidedAccountsIndex: 0,
	LogsIndex:             0,
	TouchedAccountsIndex:  0,
}

// StoreRevertKey defines the required information to revert to a previous state.
type StoreRevertKey struct {
	RefundIndex           int
	SuicidedAccountsIndex int
	LogsIndex             int
	TouchedAccountsIndex  int
}

// EphemeralStore provides in-memory state of the refund and suicided accounts
// state with the ability to revert to a previous state.
type EphemeralStore struct {
	RefundStates          []uint64
	SuicidedAccountStates []common.Address
	Logs                  []*ethtypes.Log
	TouchedAccounts       []common.Address
}

// NewEphemeralStore creates a new EphemeralStore.
func NewEphemeralStore() *EphemeralStore {
	return &EphemeralStore{
		RefundStates:          nil,
		SuicidedAccountStates: nil,
		Logs:                  nil,
		TouchedAccounts:       nil,
	}
}

// GetRevertKey returns the identifier of the current state of the store.
func (es *EphemeralStore) GetRevertKey() StoreRevertKey {
	return StoreRevertKey{
		RefundIndex:           len(es.RefundStates),
		SuicidedAccountsIndex: len(es.SuicidedAccountStates),
		LogsIndex:             len(es.Logs),
		TouchedAccountsIndex:  len(es.TouchedAccounts),
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
	es.TouchedAccounts = es.TouchedAccounts[:key.TouchedAccountsIndex]
}

func validateIndex(idx int, targetLen int, name string) error {
	if idx > targetLen {
		return fmt.Errorf(
			"invalid %s index, %d is greater than the length of the %s (%d)",
			name, idx, name, targetLen,
		)
	}

	return nil
}

func (es *EphemeralStore) ValidateRevertKey(key StoreRevertKey) error {
	validations := []struct {
		idx    int
		length int
		name   string
	}{
		{key.RefundIndex, len(es.RefundStates), "Refund"},
		{key.SuicidedAccountsIndex, len(es.SuicidedAccountStates), "SuicidedAccounts"},
		{key.LogsIndex, len(es.Logs), "Logs"},
		{key.TouchedAccountsIndex, len(es.TouchedAccounts), "TouchedAccounts"},
	}

	for _, v := range validations {
		if err := validateIndex(v.idx, v.length, v.name); err != nil {
			return err
		}
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
// Touched accounts

// SetTouched sets the given account as touched.
func (es *EphemeralStore) SetTouched(addr common.Address) {
	// If already in the list, ignore
	if es.IsTouched(addr) {
		return
	}

	es.TouchedAccounts = append(es.TouchedAccounts, addr)
}

// IsTouched returns true if the given account is touched.
func (es *EphemeralStore) IsTouched(addr common.Address) bool {
	for _, a := range es.TouchedAccounts {
		if a == addr {
			return true
		}
	}

	return false
}

// GetAllTouched returns all touched accounts.
func (es *EphemeralStore) GetAllTouched() []common.Address {
	return es.TouchedAccounts
}
