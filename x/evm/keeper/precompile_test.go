package keeper_test

import (
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/require"

	"github.com/evmos/ethermint/x/evm/keeper"
)

var (
	addr1 = common.HexToAddress("0x1000000000000000000000000000000000000000")
	addr2 = common.HexToAddress("0x2000000000000000000000000000000000000000")
	addr3 = common.HexToAddress("0x3000000000000000000000000000000000000000")
)

type account struct {
	nonce uint64
	code  []byte
}

func newAccount() *account {
	return &account{}
}

type stateDB struct {
	accounts map[common.Address]*account
}

func newStateDB() *stateDB {
	return &stateDB{
		accounts: make(map[common.Address]*account, 0),
	}
}

func (s *stateDB) GetNonce(addr common.Address) uint64 {
	account := s.getOrNewAccount(addr)
	return account.nonce
}

func (s *stateDB) GetCode(addr common.Address) []byte {
	account := s.getOrNewAccount(addr)
	return account.code
}

func (s *stateDB) SetNonce(addr common.Address, nonce uint64) {
	account := s.getOrNewAccount(addr)
	account.nonce = nonce
}

func (s *stateDB) SetCode(addr common.Address, code []byte) {
	account := s.getOrNewAccount(addr)
	account.code = code
}

func (s *stateDB) getOrNewAccount(addr common.Address) *account {
	_, ok := s.accounts[addr]
	if !ok {
		s.accounts[addr] = newAccount()
	}

	return s.accounts[addr]
}

// TestSyncEnabledPrecompiles is built using such approach:
// test case #0 - performs S0 -> S1 state transition
// test case #1 - performs S1 -> S2 state transition
// test case #n - performs Sn -> Sn+1 state transition
// it means order of test cases matters
func (suite *KeeperTestSuite) TestSyncEnabledPrecompiles() {
	testCases := []struct {
		name string
		// enabled precompiles from old state
		old []common.Address
		// enabled precompiles from new state
		new []common.Address
		// precompiles which must be uninitialized after corresponding test case
		uninitialized []common.Address
	}{
		{
			name:          "enable addr1 and addr2",
			old:           []common.Address{},
			new:           []common.Address{addr1, addr2},
			uninitialized: []common.Address{addr3},
		},
		{
			name:          "enable addr3, and disable the rest",
			old:           []common.Address{addr1, addr2},
			new:           []common.Address{addr3},
			uninitialized: []common.Address{addr1, addr2},
		},
		{
			name:          "no changes",
			old:           []common.Address{addr3},
			new:           []common.Address{addr3},
			uninitialized: []common.Address{addr1, addr2},
		},
		{
			name:          "enable all precompiles",
			old:           []common.Address{addr3},
			new:           []common.Address{addr1, addr2, addr3},
			uninitialized: []common.Address{},
		},
		{
			name:          "disable all precompiles",
			old:           []common.Address{addr1, addr2, addr3},
			new:           []common.Address{},
			uninitialized: []common.Address{addr1, addr2, addr3},
		},
	}

	for _, tc := range testCases {
		suite.Run(tc.name, func() {
			err := suite.app.EvmKeeper.SyncEnabledPrecompiles(suite.ctx, tc.new)
			suite.Require().NoError(err)

			err = keeper.ValidatePrecompilesInitialized(suite.StateDB(), tc.new)
			suite.Require().NoError(err)

			err = keeper.ValidatePrecompilesUninitialized(suite.StateDB(), tc.uninitialized)
			suite.Require().NoError(err)

			params := suite.app.EvmKeeper.GetParams(suite.ctx)
			params.EnabledPrecompiles = keeper.AddressesToHex(tc.new)
			err = suite.app.EvmKeeper.SetParams(suite.ctx, params)
			suite.Require().NoError(err)
		})
	}
}

// TestSyncEnabledPrecompiles is built using such approach:
// test case #0 - performs S0 -> S1 state transition
// test case #1 - performs S1 -> S2 state transition
// test case #n - performs Sn -> Sn+1 state transition
// it means order of test cases matters
// stateDB is reused across all test-cases
func TestSyncEnabledPrecompiles(t *testing.T) {
	stateDB := newStateDB()

	testCases := []struct {
		name          string
		old           []common.Address
		new           []common.Address
		uninitialized []common.Address
	}{
		{
			name:          "enable addr1 and addr2",
			old:           []common.Address{},
			new:           []common.Address{addr1, addr2},
			uninitialized: []common.Address{addr3},
		},
		{
			name:          "enable addr3, and disable the rest",
			old:           []common.Address{addr1, addr2},
			new:           []common.Address{addr3},
			uninitialized: []common.Address{addr1, addr2},
		},
		{
			name:          "no changes",
			old:           []common.Address{addr3},
			new:           []common.Address{addr3},
			uninitialized: []common.Address{addr1, addr2},
		},
		{
			name:          "enable all precompiles",
			old:           []common.Address{addr3},
			new:           []common.Address{addr1, addr2, addr3},
			uninitialized: []common.Address{},
		},
		{
			name:          "disable all precompiles",
			old:           []common.Address{addr1, addr2, addr3},
			new:           []common.Address{},
			uninitialized: []common.Address{addr1, addr2, addr3},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := keeper.SyncEnabledPrecompiles(stateDB, tc.old, tc.new)
			require.NoError(t, err)

			err = keeper.ValidatePrecompilesInitialized(stateDB, tc.new)
			require.NoError(t, err)

			err = keeper.ValidatePrecompilesUninitialized(stateDB, tc.uninitialized)
			require.NoError(t, err)
		})
	}
}

func TestDetermineInitializationConfig(t *testing.T) {
	testCases := []struct {
		name string
		old  []common.Address
		new  []common.Address
		cfg  *keeper.InitializationConfig
	}{
		{
			name: "enable addr1 and addr2",
			old:  []common.Address{},
			new:  []common.Address{addr1, addr2},
			cfg: &keeper.InitializationConfig{
				ValidateInitialized:   []common.Address{},
				ValidateUninitialized: []common.Address{addr1, addr2},
				Initialize:            []common.Address{addr1, addr2},
				Uninitialize:          []common.Address{},
			},
		},
		{
			name: "enable addr3, and disable the rest",
			old:  []common.Address{addr1, addr2},
			new:  []common.Address{addr3},
			cfg: &keeper.InitializationConfig{
				ValidateInitialized:   []common.Address{addr1, addr2},
				ValidateUninitialized: []common.Address{addr3},
				Initialize:            []common.Address{addr3},
				Uninitialize:          []common.Address{addr1, addr2},
			},
		},
		{
			name: "no changes",
			old:  []common.Address{addr3},
			new:  []common.Address{addr3},
			cfg: &keeper.InitializationConfig{
				ValidateInitialized:   []common.Address{addr3},
				ValidateUninitialized: []common.Address{},
				Initialize:            []common.Address{},
				Uninitialize:          []common.Address{},
			},
		},
		{
			name: "enable all precompiles",
			old:  []common.Address{addr3},
			new:  []common.Address{addr1, addr2, addr3},
			cfg: &keeper.InitializationConfig{
				ValidateInitialized:   []common.Address{addr3},
				ValidateUninitialized: []common.Address{addr1, addr2},
				Initialize:            []common.Address{addr1, addr2},
				Uninitialize:          []common.Address{},
			},
		},
		{
			name: "disable all precompiles",
			old:  []common.Address{addr1, addr2, addr3},
			new:  []common.Address{},
			cfg: &keeper.InitializationConfig{
				ValidateInitialized:   []common.Address{addr1, addr2, addr3},
				ValidateUninitialized: []common.Address{},
				Initialize:            []common.Address{},
				Uninitialize:          []common.Address{addr1, addr2, addr3},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := keeper.DetermineInitializationConfig(tc.old, tc.new)
			require.Equal(t, tc.cfg, cfg)
		})
	}
}

func TestSyncEnabledPrecompilesHelpers(t *testing.T) {
	t.Run("initialize precompiles", func(t *testing.T) {
		stateDB := newStateDB()

		require.Equal(t, uint64(0), stateDB.GetNonce(addr1))
		require.Equal(t, []byte(nil), stateDB.GetCode(addr1))

		keeper.InitializePrecompiles(stateDB, []common.Address{addr1})

		require.Equal(t, keeper.PrecompileNonce, stateDB.GetNonce(addr1))
		require.Equal(t, keeper.PrecompileCode, stateDB.GetCode(addr1))
	})

	t.Run("uninitialize precompiles", func(t *testing.T) {
		stateDB := newStateDB()

		keeper.InitializePrecompiles(stateDB, []common.Address{addr1})
		require.Equal(t, keeper.PrecompileNonce, stateDB.GetNonce(addr1))
		require.Equal(t, keeper.PrecompileCode, stateDB.GetCode(addr1))

		keeper.UninitializePrecompiles(stateDB, []common.Address{addr1})
		require.Equal(t, uint64(0), stateDB.GetNonce(addr1))
		require.Equal(t, []byte(nil), stateDB.GetCode(addr1))
	})

	t.Run("validate precompiles initialized", func(t *testing.T) {
		stateDB := newStateDB()

		err := keeper.ValidatePrecompilesInitialized(stateDB, []common.Address{addr1})
		require.ErrorContains(t, err, "is not initialized")

		keeper.InitializePrecompiles(stateDB, []common.Address{addr1})

		err = keeper.ValidatePrecompilesInitialized(stateDB, []common.Address{addr1})
		require.NoError(t, err)
	})

	t.Run("validate precompiles uninitialized", func(t *testing.T) {
		stateDB := newStateDB()

		err := keeper.ValidatePrecompilesUninitialized(stateDB, []common.Address{addr1})
		require.NoError(t, err)

		keeper.InitializePrecompiles(stateDB, []common.Address{addr1})

		err = keeper.ValidatePrecompilesUninitialized(stateDB, []common.Address{addr1})
		require.ErrorContains(t, err, "is initialized")
	})
}

func TestSetDifference(t *testing.T) {
	testCases := []struct {
		name string
		a    []common.Address
		b    []common.Address
		diff []common.Address
	}{
		{
			name: "A and B intersect, but diff isn't empty",
			a:    []common.Address{addr1, addr2},
			b:    []common.Address{addr1, addr3},
			diff: []common.Address{addr2},
		},
		{
			name: "A and B don't intersect, diff isn't empty",
			a:    []common.Address{addr1},
			b:    []common.Address{addr2, addr3},
			diff: []common.Address{addr1},
		},
		{
			name: "A is a subset of B, diff is empty",
			a:    []common.Address{addr1, addr2},
			b:    []common.Address{addr1, addr2, addr3},
			diff: []common.Address{},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			diff := keeper.SetDifference(tc.a, tc.b)
			require.Equal(t, tc.diff, diff)
		})
	}
}
