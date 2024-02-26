package statedb

import (
	"bytes"
	"math/big"

	"github.com/ethereum/go-ethereum/crypto"
)

var emptyCodeHash = crypto.Keccak256(nil)

// Account is the Ethereum consensus representation of accounts.
// These objects are stored in the storage of auth module.
type Account struct {
	Nonce    uint64
	Balance  *big.Int
	CodeHash []byte
}

// NewEmptyAccount returns an empty account.
func NewEmptyAccount() *Account {
	return &Account{
		Balance:  new(big.Int),
		CodeHash: emptyCodeHash,
	}
}

// IsContract returns if the account contains contract code.
func (acct Account) IsContract() bool {
	return !bytes.Equal(acct.CodeHash, emptyCodeHash)
}

// IsEmpty returns true if the account is empty according to EIP-161.
// An account is considered empty when it has no code and zero nonce and zero balance.
func (acct Account) IsEmpty() bool {
	return acct.Balance.Sign() == 0 && acct.Nonce == 0 && bytes.Equal(acct.CodeHash, emptyCodeHash)
}
