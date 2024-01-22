package statedb_test

import (
	"bytes"
	"errors"
	"math/big"

	storetypes "github.com/cosmos/cosmos-sdk/store/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/evmos/ethermint/x/evm/statedb"
	"github.com/evmos/ethermint/x/evm/types"
	"github.com/evmos/ethermint/x/evm/vm"
)

var (
	_             vm.StateDBKeeper = &MockKeeper{}
	errAddress    common.Address   = common.BigToAddress(big.NewInt(100))
	emptyCodeHash                  = crypto.Keccak256(nil)
)

type MockAcount struct {
	account types.StateDBAccount
	states  statedb.Storage
}

type MockKeeper struct {
	Accounts map[common.Address]MockAcount
	Codes    map[common.Hash][]byte
}

func NewMockKeeper() *MockKeeper {
	return &MockKeeper{
		Accounts: make(map[common.Address]MockAcount),
		Codes:    make(map[common.Hash][]byte),
	}
}

func (k MockKeeper) GetAccount(ctx sdk.Context, addr common.Address) *types.StateDBAccount {
	acct, ok := k.Accounts[addr]
	if !ok {
		return nil
	}
	return &acct.account
}

func (k MockKeeper) GetState(ctx sdk.Context, addr common.Address, key common.Hash) common.Hash {
	return k.Accounts[addr].states[key]
}

func (k MockKeeper) GetCode(ctx sdk.Context, codeHash common.Hash) []byte {
	return k.Codes[codeHash]
}

func (k MockKeeper) ForEachStorage(ctx sdk.Context, addr common.Address, cb func(key, value common.Hash) bool) {
	if acct, ok := k.Accounts[addr]; ok {
		for k, v := range acct.states {
			if !cb(k, v) {
				return
			}
		}
	}
}

func (k MockKeeper) SetAccount(ctx sdk.Context, addr common.Address, account types.StateDBAccount) error {
	if addr == errAddress {
		return errors.New("mock db error")
	}
	acct, exists := k.Accounts[addr]
	if exists {
		// update
		acct.account = account
		k.Accounts[addr] = acct
	} else {
		k.Accounts[addr] = MockAcount{account: account, states: make(statedb.Storage)}
	}
	return nil
}

func (k MockKeeper) SetState(ctx sdk.Context, addr common.Address, key common.Hash, value []byte) {
	if acct, ok := k.Accounts[addr]; ok {
		if len(value) == 0 {
			delete(acct.states, key)
		} else {
			acct.states[key] = common.BytesToHash(value)
		}
	}
}

func (k MockKeeper) SetCode(ctx sdk.Context, codeHash []byte, code []byte) {
	k.Codes[common.BytesToHash(codeHash)] = code
}

func (k MockKeeper) SetBalance(ctx sdk.Context, addr common.Address, amount *big.Int) error {
	if addr == errAddress {
		return errors.New("mock db error")
	}
	acct, ok := k.Accounts[addr]
	if !ok {
		return errors.New("account not found")
	}
	acct.account.Balance = amount
	k.Accounts[addr] = acct
	return nil
}

func (k MockKeeper) DeleteAccount(ctx sdk.Context, addr common.Address) error {
	if addr == errAddress {
		return errors.New("mock db error")
	}
	old := k.Accounts[addr]
	delete(k.Accounts, addr)
	if !bytes.Equal(old.account.CodeHash, emptyCodeHash) {
		delete(k.Codes, common.BytesToHash(old.account.CodeHash))
	}
	return nil
}

func (k MockKeeper) Clone() *MockKeeper {
	accounts := make(map[common.Address]MockAcount, len(k.Accounts))
	for k, v := range k.Accounts {
		accounts[k] = v
	}
	codes := make(map[common.Hash][]byte, len(k.Codes))
	for k, v := range k.Codes {
		codes[k] = v
	}
	return &MockKeeper{accounts, codes}
}

func (k MockKeeper) GetTransientKey() storetypes.StoreKey {
	return testKvStoreKey
}
