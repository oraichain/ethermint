package types

import (
	"bytes"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
)

var emptyCodeHash = crypto.Keccak256(nil)

// StateDBAccount is the Ethereum consensus representation of accounts.
// These objects are stored in the storage of auth module.
type StateDBAccount struct {
	Nonce    uint64
	Balance  *big.Int
	CodeHash []byte
}

// NewEmptyAccount returns an empty account.
func NewEmptyAccount() *StateDBAccount {
	return &StateDBAccount{
		Balance:  new(big.Int),
		CodeHash: emptyCodeHash,
	}
}

// IsContract returns if the account contains contract code.
func (acct StateDBAccount) IsContract() bool {
	return !bytes.Equal(acct.CodeHash, emptyCodeHash)
}

// TxConfig encapulates the readonly information of current tx for `StateDB`.
type TxConfig struct {
	BlockHash common.Hash // hash of current block
	TxHash    common.Hash // hash of current tx
	TxIndex   uint        // the index of current transaction
	LogIndex  uint        // the index of next log within current block
}

// NewTxConfig returns a TxConfig
func NewTxConfig(bhash, thash common.Hash, txIndex, logIndex uint) TxConfig {
	return TxConfig{
		BlockHash: bhash,
		TxHash:    thash,
		TxIndex:   txIndex,
		LogIndex:  logIndex,
	}
}

// NewEmptyTxConfig construct an empty TxConfig,
// used in context where there's no transaction, e.g. `eth_call`/`eth_estimateGas`.
func NewEmptyTxConfig(bhash common.Hash) TxConfig {
	return TxConfig{
		BlockHash: bhash,
		TxHash:    common.Hash{},
		TxIndex:   0,
		LogIndex:  0,
	}
}
