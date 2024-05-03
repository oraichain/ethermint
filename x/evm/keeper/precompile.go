package keeper

import (
	"bytes"
	"fmt"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/ethereum/go-ethereum/common"

	"github.com/evmos/ethermint/x/evm/statedb"
)

const PrecompileNonce uint64 = 1

var PrecompileCode = []byte{0x1}

type StateDB interface {
	GetNonce(addr common.Address) uint64
	GetCode(addr common.Address) []byte
	SetNonce(common.Address, uint64)
	SetCode(common.Address, []byte)
}

// InitializationConfig contains lists of contracts which has to be validated, initialized and uninitialized correspondingly.
type InitializationConfig struct {
	ValidateInitialized   []common.Address
	ValidateUninitialized []common.Address
	Initialize            []common.Address
	Uninitialize          []common.Address
}

// SyncEnabledPrecompiles is a keeper wrapper over the SyncEnabledPrecompiles function, which does most of the work.
func (k *Keeper) SyncEnabledPrecompiles(ctx sdk.Context, enabledPrecompiles []common.Address) error {
	txConfig := statedb.NewEmptyTxConfig(common.BytesToHash(ctx.HeaderHash().Bytes()))
	stateDB := statedb.New(ctx, k, txConfig)

	oldParams := k.GetParams(ctx)

	err := SyncEnabledPrecompiles(stateDB, HexToAddresses(oldParams.EnabledPrecompiles), enabledPrecompiles)
	if err != nil {
		return err
	}

	if err := stateDB.Commit(); err != nil {
		return err
	}

	return nil
}

// SyncEnabledPrecompiles takes enabled precompiles from old state and new state and performs following steps:
// - determines a list of contracts that must be validated and validates their state
// - determines a list of contracts that must be initialized and initializes them
// - determines a list of contracts that must be uninitialized and uninitializes them
func SyncEnabledPrecompiles(stateDB StateDB, old []common.Address, new []common.Address) error {
	cfg := DetermineInitializationConfig(old, new)
	return ApplyInitializationConfig(stateDB, cfg)
}

// DetermineInitializationConfig takes enabled precompiles from old state and new state and determines lists of contracts
// which has to be validated, initialized and uninitialized correspondingly.
func DetermineInitializationConfig(old []common.Address, new []common.Address) *InitializationConfig {
	return &InitializationConfig{
		ValidateInitialized:   old,
		ValidateUninitialized: SetDifference(new, old),
		Initialize:            SetDifference(new, old),
		Uninitialize:          SetDifference(old, new),
	}
}

// ApplyInitializationConfig performs precompiles initialization based on InitializationConfig.
func ApplyInitializationConfig(stateDB StateDB, cfg *InitializationConfig) error {
	if err := ValidatePrecompilesInitialized(stateDB, cfg.ValidateInitialized); err != nil {
		return err
	}
	if err := ValidatePrecompilesUninitialized(stateDB, cfg.ValidateUninitialized); err != nil {
		return err
	}

	InitializePrecompiles(stateDB, cfg.Initialize)
	UninitializePrecompiles(stateDB, cfg.Uninitialize)

	return nil
}

// ValidatePrecompilesInitialized validates that precompiles at specified addresses are initialized.
func ValidatePrecompilesInitialized(stateDB StateDB, addrs []common.Address) error {
	for _, addr := range addrs {
		nonce := stateDB.GetNonce(addr)
		code := stateDB.GetCode(addr)

		ok := nonce == PrecompileNonce && bytes.Equal(code, PrecompileCode)
		if !ok {
			return fmt.Errorf("precompile %v is not initialized, nonce: %v, code: %v", addr, nonce, code)
		}
	}

	return nil
}

// ValidatePrecompilesUninitialized validates that precompiles at specified addresses are uninitialized.
func ValidatePrecompilesUninitialized(stateDB StateDB, addrs []common.Address) error {
	for _, addr := range addrs {
		nonce := stateDB.GetNonce(addr)
		code := stateDB.GetCode(addr)

		ok := nonce == 0 && bytes.Equal(code, nil)
		if !ok {
			return fmt.Errorf("precompile %v is initialized, nonce: %v, code: %v", addr, nonce, code)
		}
	}

	return nil
}

// InitializePrecompiles initializes list of precompiles at specified addresses.
// Initialization of precompile sets non-zero nonce and non-empty code at specified address to resemble behavior of
// regular smart contract.
func InitializePrecompiles(stateDB StateDB, addrs []common.Address) {
	for _, addr := range addrs {
		// Set the nonce of the precompile's address (as is done when a contract is created) to ensure
		// that it is marked as non-empty and will not be cleaned up when the statedb is finalized.
		stateDB.SetNonce(addr, PrecompileNonce)
		// Set the code of the precompile's address to a non-zero length byte slice to ensure that the precompile
		// can be called from within Solidity contracts. Solidity adds a check before invoking a contract to ensure
		// that it does not attempt to invoke a non-existent contract.
		stateDB.SetCode(addr, PrecompileCode)
	}
}

// UninitializePrecompiles uninitializes list of precompiles at specified addresses.
// Uninitialization of precompile sets zero nonce and empty code at specified address.
func UninitializePrecompiles(stateDB StateDB, addrs []common.Address) {
	for _, addr := range addrs {
		stateDB.SetNonce(addr, 0)
		stateDB.SetCode(addr, nil)
	}
}

func HexToAddresses(hexAddrs []string) []common.Address {
	addrs := make([]common.Address, len(hexAddrs))
	for i, hexAddr := range hexAddrs {
		addrs[i] = common.HexToAddress(hexAddr)
	}

	return addrs
}

func AddressesToHex(addrs []common.Address) []string {
	hexAddrs := make([]string, len(addrs))
	for i, addr := range addrs {
		hexAddrs[i] = addr.Hex()
	}

	return hexAddrs
}

// SetDifference returns difference between two sets, example can be:
// a   : {1, 2, 3}
// b   : {1, 3}
// diff: {2}
func SetDifference(a []common.Address, b []common.Address) []common.Address {
	bMap := make(map[common.Address]struct{}, len(b))
	for _, elem := range b {
		bMap[elem] = struct{}{}
	}

	diff := make([]common.Address, 0)
	for _, elem := range a {
		if _, ok := bMap[elem]; !ok {
			diff = append(diff, elem)
		}
	}

	return diff
}
