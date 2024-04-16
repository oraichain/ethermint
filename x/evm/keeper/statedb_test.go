package keeper_test

import (
	"fmt"
	"math/big"
	"time"

	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	cryptotypes "github.com/cosmos/cosmos-sdk/crypto/types"
	"github.com/cosmos/cosmos-sdk/store/iavl"
	"github.com/cosmos/cosmos-sdk/store/prefix"
	sdk "github.com/cosmos/cosmos-sdk/types"
	authsigning "github.com/cosmos/cosmos-sdk/x/auth/signing"
	authtx "github.com/cosmos/cosmos-sdk/x/auth/tx"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	vestingtypes "github.com/cosmos/cosmos-sdk/x/auth/vesting/types"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"

	"github.com/ethereum/go-ethereum/common"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/evmos/ethermint/crypto/ethsecp256k1"
	"github.com/evmos/ethermint/tests"
	etherminttypes "github.com/evmos/ethermint/types"
	"github.com/evmos/ethermint/x/evm/statedb"
	"github.com/evmos/ethermint/x/evm/types"
)

func (suite *KeeperTestSuite) TestCreateAccount() {
	testCases := []struct {
		name     string
		addr     common.Address
		malleate func(*statedb.StateDB, common.Address)
		callback func(*statedb.StateDB, common.Address)
	}{
		{
			"reset account (keep balance)",
			suite.Address,
			func(vmdb *statedb.StateDB, addr common.Address) {
				vmdb.AddBalance(addr, big.NewInt(100))
				suite.Require().NotZero(vmdb.GetBalance(addr).Int64())
			},
			func(vmdb *statedb.StateDB, addr common.Address) {
				suite.Require().Equal(vmdb.GetBalance(addr).Int64(), int64(100))
			},
		},
		{
			"create account",
			tests.GenerateAddress(),
			func(vmdb *statedb.StateDB, addr common.Address) {
				suite.Require().False(vmdb.Exist(addr))
			},
			func(vmdb *statedb.StateDB, addr common.Address) {
				suite.Require().True(vmdb.Exist(addr))
			},
		},
	}

	for _, tc := range testCases {
		suite.Run(tc.name, func() {
			vmdb := suite.StateDB()
			tc.malleate(vmdb, tc.addr)
			vmdb.CreateAccount(tc.addr)
			tc.callback(vmdb, tc.addr)
		})
	}
}

func (suite *KeeperTestSuite) TestAddBalance() {
	testCases := []struct {
		name   string
		amount *big.Int
		isNoOp bool
	}{
		{
			"positive amount",
			big.NewInt(100),
			false,
		},
		{
			"zero amount",
			big.NewInt(0),
			true,
		},
		{
			"negative amount",
			big.NewInt(-1),
			// Pre-cache-ctx implementation allowed negative amounts, which
			// seems to be consistent with go-ethereum's implementation
			true,
		},
	}

	for _, tc := range testCases {
		suite.Run(tc.name, func() {
			vmdb := suite.StateDB()
			prev := vmdb.GetBalance(suite.Address)
			vmdb.AddBalance(suite.Address, tc.amount)
			post := vmdb.GetBalance(suite.Address)

			if tc.isNoOp {
				suite.Require().Equal(prev.Int64(), post.Int64())
			} else {
				suite.Require().Equal(new(big.Int).Add(prev, tc.amount).Int64(), post.Int64())
			}
		})
	}
}

func (suite *KeeperTestSuite) TestSubBalance() {
	testCases := []struct {
		name     string
		amount   *big.Int
		malleate func(*statedb.StateDB)
		isNoOp   bool
	}{
		{
			"positive amount, below zero",
			big.NewInt(100),
			func(*statedb.StateDB) {},
			true,
		},
		{
			"positive amount, above zero",
			big.NewInt(50),
			func(vmdb *statedb.StateDB) {
				vmdb.AddBalance(suite.Address, big.NewInt(100))
			},
			false,
		},
		{
			"zero amount",
			big.NewInt(0),
			func(*statedb.StateDB) {},
			true,
		},
		{
			"negative amount",
			big.NewInt(-1),
			func(*statedb.StateDB) {},
			false,
		},
	}

	for _, tc := range testCases {
		suite.Run(tc.name, func() {
			vmdb := suite.StateDB()
			tc.malleate(vmdb)

			prev := vmdb.GetBalance(suite.Address)
			vmdb.SubBalance(suite.Address, tc.amount)
			post := vmdb.GetBalance(suite.Address)

			if tc.isNoOp {
				suite.Require().Equal(prev.Int64(), post.Int64())
			} else {
				suite.Require().Equal(new(big.Int).Sub(prev, tc.amount).Int64(), post.Int64())
			}
		})
	}
}

func (suite *KeeperTestSuite) TestGetNonce() {
	testCases := []struct {
		name          string
		address       common.Address
		expectedNonce uint64
		malleate      func(*statedb.StateDB)
	}{
		{
			"account not found",
			tests.GenerateAddress(),
			0,
			func(*statedb.StateDB) {},
		},
		{
			"existing account",
			suite.Address,
			1,
			func(vmdb *statedb.StateDB) {
				vmdb.SetNonce(suite.Address, 1)
			},
		},
	}

	for _, tc := range testCases {
		suite.Run(tc.name, func() {
			vmdb := suite.StateDB()
			tc.malleate(vmdb)

			nonce := vmdb.GetNonce(tc.address)
			suite.Require().Equal(tc.expectedNonce, nonce)
		})
	}
}

func (suite *KeeperTestSuite) TestSetNonce() {
	testCases := []struct {
		name     string
		address  common.Address
		nonce    uint64
		malleate func()
	}{
		{
			"new account",
			tests.GenerateAddress(),
			10,
			func() {},
		},
		{
			"existing account",
			suite.Address,
			99,
			func() {},
		},
	}

	for _, tc := range testCases {
		suite.Run(tc.name, func() {
			vmdb := suite.StateDB()
			vmdb.SetNonce(tc.address, tc.nonce)
			nonce := vmdb.GetNonce(tc.address)
			suite.Require().Equal(tc.nonce, nonce)
		})
	}
}

func (suite *KeeperTestSuite) TestSetAccount() {
	baseAddr := tests.GenerateAddress()
	baseAcc := &authtypes.BaseAccount{Address: sdk.AccAddress(baseAddr.Bytes()).String()}
	ethAddr := tests.GenerateAddress()
	ethAcc := &etherminttypes.EthAccount{BaseAccount: &authtypes.BaseAccount{Address: sdk.AccAddress(ethAddr.Bytes()).String()}, CodeHash: common.BytesToHash(types.EmptyCodeHash).String()}
	vestingAddr := tests.GenerateAddress()
	vestingAcc := vestingtypes.NewBaseVestingAccount(&authtypes.BaseAccount{Address: sdk.AccAddress(vestingAddr.Bytes()).String()}, sdk.NewCoins(), time.Now().Unix())

	testCases := []struct {
		name        string
		address     common.Address
		account     statedb.Account
		expectedErr error
	}{
		{
			"new account, non-contract account",
			tests.GenerateAddress(),
			statedb.Account{
				Nonce: 10,
				// Balance:,
				CodeHash: types.EmptyCodeHash,
			},
			nil,
		},
		{
			"new account, contract account",
			tests.GenerateAddress(),
			statedb.Account{
				Nonce: 10,
				// Balance: big.NewInt(100)
				CodeHash: crypto.Keccak256Hash([]byte("some code hash")).Bytes(),
			},
			nil,
		},
		{
			"existing eth account, non-contract account",
			ethAddr,
			statedb.Account{
				Nonce: 10,
				// Balance: big.NewInt(1),
				CodeHash: types.EmptyCodeHash,
			},
			nil,
		},
		{
			"existing eth account, contract account",
			ethAddr,
			statedb.Account{
				Nonce: 10,
				// Balance: /* big.NewInt(0),*/,
				CodeHash: crypto.Keccak256Hash([]byte("some code hash")).Bytes(),
			},
			nil,
		},
		{
			"existing base account, non-contract account",
			baseAddr,
			statedb.Account{
				Nonce: 10,
				// Balance: /* big.NewInt(10),*/,
				CodeHash: types.EmptyCodeHash,
			},
			nil,
		},
		{
			"existing base account, contract account",
			baseAddr,
			statedb.Account{
				Nonce: 10,
				// Balance: /* big.NewInt(99),*/,
				CodeHash: crypto.Keccak256Hash([]byte("some code hash")).Bytes(),
			},
			nil,
		},
		{
			"existing vesting account, non-contract account",
			vestingAddr,
			statedb.Account{
				Nonce: 10,
				// Balance: /* big.NewInt(1000),*/,
				CodeHash: types.EmptyCodeHash,
			},
			nil,
		},
		{
			"existing vesting account, contract account",
			vestingAddr,
			statedb.Account{
				Nonce: 10,
				// Balance: /* big.NewInt(1001),*/,
				CodeHash: crypto.Keccak256Hash([]byte("some code hash")).Bytes(),
			},
			types.ErrInvalidAccount,
		},
	}

	for _, tc := range testCases {
		suite.Run(tc.name, func() {
			if tc.address == baseAddr {
				suite.App.AccountKeeper.SetAccount(suite.Ctx, baseAcc)
			}
			if tc.address == ethAddr {
				suite.App.AccountKeeper.SetAccount(suite.Ctx, ethAcc)
			}
			if tc.address == vestingAddr {
				suite.App.AccountKeeper.SetAccount(suite.Ctx, vestingAcc)
			}

			vmdb := suite.StateDB()
			err := vmdb.Keeper().SetAccount(suite.Ctx, tc.address, tc.account)

			if tc.expectedErr == nil {
				suite.Require().NoError(err)
			} else {
				suite.Require().Error(tc.expectedErr)
				return
			}

			nonce := vmdb.GetNonce(tc.address)
			suite.Equal(nonce, tc.account.Nonce, "expected nonce to be set")

			hash := vmdb.GetCodeHash(tc.address)
			suite.Equal(common.BytesToHash(tc.account.CodeHash), hash, "expected code hash to be set")

			balance := vmdb.GetBalance(tc.address)
			suite.Equal(big.NewInt(0), balance, "balance is not set from SetAccount")
		})
	}
}

func (suite *KeeperTestSuite) TestGetCodeHash() {
	addr := tests.GenerateAddress()
	baseAcc := &authtypes.BaseAccount{Address: sdk.AccAddress(addr.Bytes()).String()}
	suite.App.AccountKeeper.SetAccount(suite.Ctx, baseAcc)

	testCases := []struct {
		name     string
		address  common.Address
		expHash  common.Hash
		malleate func(*statedb.StateDB)
	}{
		{
			"account not found",
			tests.GenerateAddress(),
			common.Hash{},
			func(*statedb.StateDB) {},
		},
		{
			"account not EthAccount type, EmptyCodeHash",
			addr,
			common.BytesToHash(types.EmptyCodeHash),
			func(*statedb.StateDB) {},
		},
		{
			"existing account",
			suite.Address,
			crypto.Keccak256Hash([]byte("codeHash")),
			func(vmdb *statedb.StateDB) {
				vmdb.SetCode(suite.Address, []byte("codeHash"))
			},
		},
	}

	for _, tc := range testCases {
		suite.Run(tc.name, func() {
			vmdb := suite.StateDB()
			tc.malleate(vmdb)

			hash := vmdb.GetCodeHash(tc.address)
			suite.Require().Equal(tc.expHash, hash)
		})
	}
}

func (suite *KeeperTestSuite) TestSetCode() {
	addr := tests.GenerateAddress()
	baseAcc := &authtypes.BaseAccount{Address: sdk.AccAddress(addr.Bytes()).String()}
	suite.App.AccountKeeper.SetAccount(suite.Ctx, baseAcc)

	testCases := []struct {
		name    string
		address common.Address
		code    []byte
		isNoOp  bool
	}{
		{
			"account not found",
			tests.GenerateAddress(),
			[]byte("code"),
			false,
		},
		{
			"account not EthAccount type",
			addr,
			nil,
			true,
		},
		{
			"existing account",
			suite.Address,
			[]byte("code"),
			false,
		},
		{
			"existing account, code deleted from store",
			suite.Address,
			nil,
			false,
		},
	}

	for _, tc := range testCases {
		suite.Run(tc.name, func() {
			vmdb := suite.StateDB()
			prev := vmdb.GetCode(tc.address)
			vmdb.SetCode(tc.address, tc.code)
			post := vmdb.GetCode(tc.address)

			if tc.isNoOp {
				suite.Require().Equal(prev, post)
			} else {
				suite.Require().Equal(tc.code, post)
			}

			suite.Require().Equal(len(post), vmdb.GetCodeSize(tc.address))
		})
	}
}

func (suite *KeeperTestSuite) TestKeeperSetCode() {
	addr := tests.GenerateAddress()
	baseAcc := &authtypes.BaseAccount{Address: sdk.AccAddress(addr.Bytes()).String()}
	suite.App.AccountKeeper.SetAccount(suite.Ctx, baseAcc)

	testCases := []struct {
		name     string
		codeHash []byte
		code     []byte
	}{
		{
			"set code",
			[]byte("codeHash"),
			[]byte("this is the code"),
		},
		{
			"delete code",
			[]byte("codeHash"),
			nil,
		},
	}

	for _, tc := range testCases {
		suite.Run(tc.name, func() {
			suite.App.EvmKeeper.SetCode(suite.Ctx, tc.codeHash, tc.code)
			key := suite.App.GetKey(types.StoreKey)
			store := prefix.NewStore(suite.Ctx.KVStore(key), types.KeyPrefixCode)
			code := store.Get(tc.codeHash)

			suite.Require().Equal(tc.code, code)
		})
	}
}

func (suite *KeeperTestSuite) TestRefund() {
	testCases := []struct {
		name      string
		malleate  func(*statedb.StateDB)
		expRefund uint64
		expPanic  bool
	}{
		{
			"success - add and subtract refund",
			func(vmdb *statedb.StateDB) {
				vmdb.AddRefund(11)
			},
			1,
			false,
		},
		{
			"fail - subtract amount > current refund",
			func(*statedb.StateDB) {
			},
			0,
			true,
		},
	}

	for _, tc := range testCases {
		suite.Run(tc.name, func() {
			vmdb := suite.StateDB()
			tc.malleate(vmdb)

			if tc.expPanic {
				suite.Require().Panics(func() { vmdb.SubRefund(10) })
			} else {
				vmdb.SubRefund(10)
				suite.Require().Equal(tc.expRefund, vmdb.GetRefund())
			}
		})
	}
}

func (suite *KeeperTestSuite) TestState() {
	testCases := []struct {
		name       string
		key, value common.Hash
	}{
		{
			"set state - delete from store",
			common.BytesToHash([]byte("key")),
			common.Hash{},
		},
		{
			"set state - update value",
			common.BytesToHash([]byte("key")),
			common.BytesToHash([]byte("value")),
		},
	}

	for _, tc := range testCases {
		suite.Run(tc.name, func() {
			vmdb := suite.StateDB()
			vmdb.SetState(suite.Address, tc.key, tc.value)
			value := vmdb.GetState(suite.Address, tc.key)
			suite.Require().Equal(tc.value, value)
		})
	}
}

func (suite *KeeperTestSuite) TestSetState_Delete() {
	// Set state
	key := common.BytesToHash([]byte("key"))
	value := common.BytesToHash([]byte("value"))
	suite.App.EvmKeeper.SetState(suite.Ctx, suite.Address, key, value)

	// Check store if exists
	storeKey := suite.App.GetKey(types.StoreKey)
	store := prefix.NewStore(suite.Ctx.KVStore(storeKey), types.AddressStoragePrefix(suite.Address))

	suite.Require().True(store.Has(key.Bytes()), "key/value should be set in store")

	// Set state with empty value to delete
	suite.App.EvmKeeper.SetState(suite.Ctx, suite.Address, key, common.Hash{})

	// Check store if deleted
	suite.Require().False(store.Has(key.Bytes()), "key/value should be deleted from store")
}

func (suite *KeeperTestSuite) TestCommittedState() {
	key := common.BytesToHash([]byte("key"))
	value1 := common.BytesToHash([]byte("value1"))
	value2 := common.BytesToHash([]byte("value2"))

	vmdb := suite.StateDB()
	vmdb.SetState(suite.Address, key, value1)
	vmdb.Commit()

	vmdb = suite.StateDB()
	vmdb.SetState(suite.Address, key, value2)
	tmp := vmdb.GetState(suite.Address, key)
	suite.Require().Equal(value2, tmp)
	tmp = vmdb.GetCommittedState(suite.Address, key)
	suite.Require().Equal(value1, tmp)
	vmdb.Commit()

	vmdb = suite.StateDB()
	tmp = vmdb.GetCommittedState(suite.Address, key)
	suite.Require().Equal(value2, tmp)
}

func (suite *KeeperTestSuite) TestSuicide() {
	code := []byte("code")
	db := suite.StateDB()
	// Add code to account
	db.SetCode(suite.Address, code)
	suite.Require().Equal(code, db.GetCode(suite.Address))
	// Add state to account
	for i := 0; i < 5; i++ {
		db.SetState(suite.Address, common.BytesToHash([]byte(fmt.Sprintf("key%d", i))), common.BytesToHash([]byte(fmt.Sprintf("value%d", i))))
	}

	suite.Require().NoError(db.Commit())
	db = suite.StateDB()

	// Generate 2nd address
	privkey, _ := ethsecp256k1.GenerateKey()
	key, err := privkey.ToECDSA()
	suite.Require().NoError(err)
	addr2 := crypto.PubkeyToAddress(key.PublicKey)

	// Add code and state to account 2
	db.SetCode(addr2, code)
	suite.Require().Equal(code, db.GetCode(addr2))
	for i := 0; i < 5; i++ {
		db.SetState(addr2, common.BytesToHash([]byte(fmt.Sprintf("key%d", i))), common.BytesToHash([]byte(fmt.Sprintf("value%d", i))))
	}

	// Call Suicide
	suite.Require().Equal(true, db.Suicide(suite.Address))

	// Check suicided is marked
	suite.Require().Equal(true, db.HasSuicided(suite.Address))

	// Commit state
	suite.Require().NoError(db.Commit())
	db = suite.StateDB()

	// Check code is deleted
	suite.Require().Nil(db.GetCode(suite.Address))
	// Check state is deleted
	var storage types.Storage
	suite.App.EvmKeeper.ForEachStorage(suite.Ctx, suite.Address, func(key, value common.Hash) bool {
		storage = append(storage, types.NewState(key, value))
		return true
	})
	suite.Require().Equal(0, len(storage))

	// Check account is deleted
	suite.Require().Equal(common.Hash{}, db.GetCodeHash(suite.Address))

	// Check code is still present in addr2 and suicided is false
	suite.Require().NotNil(db.GetCode(addr2))
	suite.Require().Equal(false, db.HasSuicided(addr2))
}

func (suite *KeeperTestSuite) TestExist() {
	testCases := []struct {
		name     string
		address  common.Address
		malleate func(*statedb.StateDB)
		exists   bool
	}{
		{"success, account exists", suite.Address, func(*statedb.StateDB) {}, true},
		{"success, has suicided", suite.Address, func(vmdb *statedb.StateDB) {
			vmdb.Suicide(suite.Address)
		}, true},
		{"success, account doesn't exist", tests.GenerateAddress(), func(*statedb.StateDB) {}, false},
	}

	for _, tc := range testCases {
		suite.Run(tc.name, func() {
			vmdb := suite.StateDB()
			tc.malleate(vmdb)

			suite.Require().Equal(tc.exists, vmdb.Exist(tc.address))
		})
	}
}

func (suite *KeeperTestSuite) TestEmpty() {
	testCases := []struct {
		name     string
		address  common.Address
		malleate func(*statedb.StateDB)
		empty    bool
	}{
		{"empty, account exists", suite.Address, func(*statedb.StateDB) {}, true},
		{
			"not empty, positive balance",
			suite.Address,
			func(vmdb *statedb.StateDB) { vmdb.AddBalance(suite.Address, big.NewInt(100)) },
			false,
		},
		{"empty, account doesn't exist", tests.GenerateAddress(), func(*statedb.StateDB) {}, true},
	}

	for _, tc := range testCases {
		suite.Run(tc.name, func() {
			suite.SetupTest()
			vmdb := suite.StateDB()
			tc.malleate(vmdb)

			suite.Require().Equal(tc.empty, vmdb.Empty(tc.address))
		})
	}
}

func (suite *KeeperTestSuite) TestSnapshot() {
	key := common.BytesToHash([]byte("key"))
	value1 := common.BytesToHash([]byte("value1"))
	value2 := common.BytesToHash([]byte("value2"))

	testCases := []struct {
		name     string
		malleate func(*statedb.StateDB)
	}{
		{"simple revert", func(vmdb *statedb.StateDB) {
			revision := vmdb.Snapshot()
			suite.Require().Zero(revision)

			vmdb.SetState(suite.Address, key, value1)
			suite.Require().Equal(value1, vmdb.GetState(suite.Address, key))

			vmdb.RevertToSnapshot(revision)

			// reverted
			suite.Require().Equal(common.Hash{}, vmdb.GetState(suite.Address, key))
		}},
		{"nested snapshot/revert", func(vmdb *statedb.StateDB) {
			revision1 := vmdb.Snapshot()
			suite.Require().Zero(revision1)

			vmdb.SetState(suite.Address, key, value1)

			revision2 := vmdb.Snapshot()

			vmdb.SetState(suite.Address, key, value2)
			suite.Require().Equal(value2, vmdb.GetState(suite.Address, key))

			vmdb.RevertToSnapshot(revision2)
			suite.Require().Equal(value1, vmdb.GetState(suite.Address, key))

			vmdb.RevertToSnapshot(revision1)
			suite.Require().Equal(common.Hash{}, vmdb.GetState(suite.Address, key))
		}},
		{"jump revert", func(vmdb *statedb.StateDB) {
			revision1 := vmdb.Snapshot()
			vmdb.SetState(suite.Address, key, value1)
			vmdb.Snapshot()
			vmdb.SetState(suite.Address, key, value2)
			vmdb.RevertToSnapshot(revision1)
			suite.Require().Equal(common.Hash{}, vmdb.GetState(suite.Address, key))
		}},
	}

	for _, tc := range testCases {
		suite.Run(tc.name, func() {
			suite.SetupTest()
			vmdb := suite.StateDB()
			tc.malleate(vmdb)
		})
	}
}

func (suite *KeeperTestSuite) CreateTestTx(msg *types.MsgEthereumTx, priv cryptotypes.PrivKey) authsigning.Tx {
	option, err := codectypes.NewAnyWithValue(&types.ExtensionOptionsEthereumTx{})
	suite.Require().NoError(err)

	txBuilder := suite.ClientCtx.TxConfig.NewTxBuilder()
	builder, ok := txBuilder.(authtx.ExtensionOptionsTxBuilder)
	suite.Require().True(ok)

	builder.SetExtensionOptions(option)

	err = msg.Sign(suite.EthSigner, tests.NewSigner(priv))
	suite.Require().NoError(err)

	err = txBuilder.SetMsgs(msg)
	suite.Require().NoError(err)

	return txBuilder.GetTx()
}

func (suite *KeeperTestSuite) TestAddLog() {
	addr, privKey := tests.NewAddrKey()
	msg := types.NewTx(big.NewInt(1), 0, &suite.Address, big.NewInt(1), 100000, big.NewInt(1), nil, nil, []byte("test"), nil)
	msg.From = addr.Hex()

	tx := suite.CreateTestTx(msg, privKey)
	msg, _ = tx.GetMsgs()[0].(*types.MsgEthereumTx)
	txHash := msg.AsTransaction().Hash()

	msg2 := types.NewTx(big.NewInt(1), 1, &suite.Address, big.NewInt(1), 100000, big.NewInt(1), nil, nil, []byte("test"), nil)
	msg2.From = addr.Hex()

	tx2 := suite.CreateTestTx(msg2, privKey)
	msg2, _ = tx2.GetMsgs()[0].(*types.MsgEthereumTx)

	msg3 := types.NewTx(big.NewInt(1), 0, &suite.Address, big.NewInt(1), 100000, nil, big.NewInt(1), big.NewInt(1), []byte("test"), nil)
	msg3.From = addr.Hex()

	tx3 := suite.CreateTestTx(msg3, privKey)
	msg3, _ = tx3.GetMsgs()[0].(*types.MsgEthereumTx)
	txHash3 := msg3.AsTransaction().Hash()

	msg4 := types.NewTx(big.NewInt(1), 1, &suite.Address, big.NewInt(1), 100000, nil, big.NewInt(1), big.NewInt(1), []byte("test"), nil)
	msg4.From = addr.Hex()

	tx4 := suite.CreateTestTx(msg4, privKey)
	msg4, _ = tx4.GetMsgs()[0].(*types.MsgEthereumTx)

	testCases := []struct {
		name        string
		hash        common.Hash
		log, expLog *ethtypes.Log // pre and post populating log fields
		malleate    func(*statedb.StateDB)
	}{
		{
			"tx hash from message",
			txHash,
			&ethtypes.Log{
				Address: addr,
				Topics:  make([]common.Hash, 0),
			},
			&ethtypes.Log{
				Address: addr,
				TxHash:  txHash,
				Topics:  make([]common.Hash, 0),
			},
			func(*statedb.StateDB) {},
		},
		{
			"dynamicfee tx hash from message",
			txHash3,
			&ethtypes.Log{
				Address: addr,
				Topics:  make([]common.Hash, 0),
			},
			&ethtypes.Log{
				Address: addr,
				TxHash:  txHash3,
				Topics:  make([]common.Hash, 0),
			},
			func(*statedb.StateDB) {},
		},
	}

	for _, tc := range testCases {
		suite.Run(tc.name, func() {
			suite.SetupTest()
			vmdb := statedb.New(suite.Ctx, suite.App.EvmKeeper, statedb.NewTxConfig(
				common.BytesToHash(suite.Ctx.HeaderHash().Bytes()),
				tc.hash,
				0, 0,
			))
			tc.malleate(vmdb)

			vmdb.AddLog(tc.log)
			logs := vmdb.Logs()
			suite.Require().Equal(1, len(logs))
			suite.Require().Equal(tc.expLog, logs[0])
		})
	}
}

func (suite *KeeperTestSuite) TestPrepareAccessList() {
	dest := tests.GenerateAddress()
	precompiles := []common.Address{tests.GenerateAddress(), tests.GenerateAddress()}
	accesses := ethtypes.AccessList{
		{Address: tests.GenerateAddress(), StorageKeys: []common.Hash{common.BytesToHash([]byte("key"))}},
		{Address: tests.GenerateAddress(), StorageKeys: []common.Hash{common.BytesToHash([]byte("key1"))}},
	}

	vmdb := suite.StateDB()
	vmdb.PrepareAccessList(suite.Address, &dest, precompiles, accesses)

	suite.Require().True(vmdb.AddressInAccessList(suite.Address))
	suite.Require().True(vmdb.AddressInAccessList(dest))

	for _, precompile := range precompiles {
		suite.Require().True(vmdb.AddressInAccessList(precompile))
	}

	for _, access := range accesses {
		for _, key := range access.StorageKeys {
			addrOK, slotOK := vmdb.SlotInAccessList(access.Address, key)
			suite.Require().True(addrOK, access.Address.Hex())
			suite.Require().True(slotOK, key.Hex())
		}
	}
}

func (suite *KeeperTestSuite) TestAddAddressToAccessList() {
	testCases := []struct {
		name string
		addr common.Address
	}{
		{"new address", suite.Address},
		{"existing address", suite.Address},
	}

	for _, tc := range testCases {
		suite.Run(tc.name, func() {
			vmdb := suite.StateDB()
			vmdb.AddAddressToAccessList(tc.addr)
			addrOk := vmdb.AddressInAccessList(tc.addr)
			suite.Require().True(addrOk, tc.addr.Hex())
		})
	}
}

func (suite *KeeperTestSuite) AddSlotToAccessList() {
	testCases := []struct {
		name string
		addr common.Address
		slot common.Hash
	}{
		{"new address and slot (1)", tests.GenerateAddress(), common.BytesToHash([]byte("hash"))},
		{"new address and slot (2)", suite.Address, common.Hash{}},
		{"existing address and slot", suite.Address, common.Hash{}},
		{"existing address, new slot", suite.Address, common.BytesToHash([]byte("hash"))},
	}

	for _, tc := range testCases {
		suite.Run(tc.name, func() {
			vmdb := suite.StateDB()
			vmdb.AddSlotToAccessList(tc.addr, tc.slot)
			addrOk, slotOk := vmdb.SlotInAccessList(tc.addr, tc.slot)
			suite.Require().True(addrOk, tc.addr.Hex())
			suite.Require().True(slotOk, tc.slot.Hex())
		})
	}
}

// FIXME skip for now
func (suite *KeeperTestSuite) _TestForEachStorage() {
	var storage types.Storage

	testCase := []struct {
		name      string
		malleate  func(*statedb.StateDB)
		callback  func(key, value common.Hash) (stop bool)
		expValues []common.Hash
	}{
		{
			"aggregate state",
			func(vmdb *statedb.StateDB) {
				for i := 0; i < 5; i++ {
					vmdb.SetState(suite.Address, common.BytesToHash([]byte(fmt.Sprintf("key%d", i))), common.BytesToHash([]byte(fmt.Sprintf("value%d", i))))
				}
			},
			func(key, value common.Hash) bool {
				storage = append(storage, types.NewState(key, value))
				return true
			},
			[]common.Hash{
				common.BytesToHash([]byte("value0")),
				common.BytesToHash([]byte("value1")),
				common.BytesToHash([]byte("value2")),
				common.BytesToHash([]byte("value3")),
				common.BytesToHash([]byte("value4")),
			},
		},
		{
			"filter state",
			func(vmdb *statedb.StateDB) {
				vmdb.SetState(suite.Address, common.BytesToHash([]byte("key")), common.BytesToHash([]byte("value")))
				vmdb.SetState(suite.Address, common.BytesToHash([]byte("filterkey")), common.BytesToHash([]byte("filtervalue")))
			},
			func(key, value common.Hash) bool {
				if value == common.BytesToHash([]byte("filtervalue")) {
					storage = append(storage, types.NewState(key, value))
					return false
				}
				return true
			},
			[]common.Hash{
				common.BytesToHash([]byte("filtervalue")),
			},
		},
	}

	for _, tc := range testCase {
		suite.Run(tc.name, func() {
			suite.SetupTest() // reset
			vmdb := suite.StateDB()
			tc.malleate(vmdb)

			err := vmdb.ForEachStorage(suite.Address, tc.callback)
			suite.Require().NoError(err)
			suite.Require().Equal(len(tc.expValues), len(storage), fmt.Sprintf("Expected values:\n%v\nStorage Values\n%v", tc.expValues, storage))

			vals := make([]common.Hash, len(storage))
			for i := range storage {
				vals[i] = common.HexToHash(storage[i].Value)
			}

			// TODO: not sure why Equals fails
			suite.Require().ElementsMatch(tc.expValues, vals)
		})
		storage = types.Storage{}
	}
}

func (suite *KeeperTestSuite) TestSetBalance() {
	amount := big.NewInt(-10)

	testCases := []struct {
		name     string
		addr     common.Address
		malleate func()
		expErr   bool
	}{
		{
			"address without funds - invalid amount",
			suite.Address,
			func() {},
			true,
		},
		{
			"mint to address",
			suite.Address,
			func() {
				amount = big.NewInt(100)
			},
			false,
		},
		{
			"burn from address",
			suite.Address,
			func() {
				amount = big.NewInt(60)
			},
			false,
		},
		{
			"address with funds - invalid amount",
			suite.Address,
			func() {
				amount = big.NewInt(-10)
			},
			true,
		},
	}

	for _, tc := range testCases {
		suite.Run(tc.name, func() {
			suite.SetupTest()
			tc.malleate()
			err := suite.App.EvmKeeper.SetBalance(suite.Ctx, tc.addr, amount)
			if tc.expErr {
				suite.Require().Error(err)
			} else {
				balance := suite.App.EvmKeeper.GetBalance(suite.Ctx, tc.addr)
				suite.Require().NoError(err)
				suite.Require().Equal(amount, balance)
			}
		})
	}
}

func (suite *KeeperTestSuite) TestDeleteAccount() {
	supply := big.NewInt(100)
	contractAddr := suite.DeployTestContract(suite.T(), suite.Address, supply)

	testCases := []struct {
		name   string
		addr   common.Address
		expErr bool
	}{
		{
			"remove address",
			suite.Address,
			false,
		},
		{
			"remove unexistent address - returns nil error",
			common.HexToAddress("unexistent_address"),
			false,
		},
		{
			"remove deployed contract",
			contractAddr,
			false,
		},
	}

	for _, tc := range testCases {
		suite.Run(tc.name, func() {
			suite.SetupTest()
			err := suite.App.EvmKeeper.DeleteAccount(suite.Ctx, tc.addr)
			if tc.expErr {
				suite.Require().Error(err)
			} else {
				suite.Require().NoError(err)
				balance := suite.App.EvmKeeper.GetBalance(suite.Ctx, tc.addr)
				suite.Require().Equal(new(big.Int), balance)
			}
		})
	}
}

func (suite *KeeperTestSuite) TestUnsetBalanceChange() {
	// init non-zero balance
	db := statedb.New(suite.Ctx, suite.App.EvmKeeper, emptyTxConfig)
	db.AddBalance(suite.Address, big.NewInt(100))
	suite.Require().NoError(db.Commit())

	suite.Commit()

	// No-op change
	db = statedb.New(suite.Ctx, suite.App.EvmKeeper, emptyTxConfig)

	db.AddBalance(suite.Address, big.NewInt(100))
	suite.Require().NotZero(db.GetBalance(suite.Address).Int64())

	db.SubBalance(suite.Address, big.NewInt(10))
	db.SubBalance(suite.Address, big.NewInt(90))

	suite.Require().Equal(int64(100), db.GetBalance(suite.Address).Int64())

	// Currently doesn't actually test if the state was removed, but just if it
	// doesn't error
	suite.Require().NoError(db.Commit())

	suite.Commit()

	store := suite.App.CommitMultiStore().GetStore(suite.App.GetKey(banktypes.StoreKey))
	iavlStore := store.(*iavl.Store)

	commitID1 := iavlStore.LastCommitID()
	suite.T().Logf("commitID: %x", commitID1.Hash)
}
