package statedb

import (
	"github.com/cosmos/cosmos-sdk/store/prefix"
	storetypes "github.com/cosmos/cosmos-sdk/store/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/ethereum/go-ethereum/common"
)

var (
	AccessListAddressKey     = []byte{0x01} // common.Address
	AccessListAddressSlotKey = []byte{0x02} // (common.Address, common.Hash)

	RefundKey   = []byte{0x03}
	SuicidedKey = []byte{0x04}
)

type Store struct {
	key storetypes.StoreKey
}

func NewStateDBStore(storeKey storetypes.StoreKey) *Store {
	return &Store{
		key: storeKey,
	}
}

// AddRefund adds a refund to the store.
func (ls *Store) AddRefund(ctx sdk.Context, gas uint64) {
	store := prefix.NewStore(ctx.KVStore(ls.key), RefundKey)
	// Add to existing refund
	bz := store.Get(RefundKey)
	if bz != nil {
		gas += sdk.BigEndianToUint64(bz)
	}

	store.Set(RefundKey, sdk.Uint64ToBigEndian(gas))
}

func (ls *Store) SubRefund(ctx sdk.Context, gas uint64) {
	store := prefix.NewStore(ctx.KVStore(ls.key), RefundKey)
	// Subtract from existing refund
	bz := store.Get(RefundKey)
	if bz == nil {
		panic("no refund to subtract from")
	}

	refund := sdk.BigEndianToUint64(bz)

	if refund < gas {
		panic("refund is less than gas")
	}

	gas = refund - gas
	store.Set(RefundKey, sdk.Uint64ToBigEndian(gas))
}

func (ls *Store) GetRefund(ctx sdk.Context) uint64 {
	store := prefix.NewStore(ctx.KVStore(ls.key), RefundKey)
	bz := store.Get(RefundKey)
	if bz == nil {
		return 0
	}
	return sdk.BigEndianToUint64(bz)
}

func (ls *Store) SetAccountSuicided(ctx sdk.Context, addr common.Address) {
	store := prefix.NewStore(ctx.KVStore(ls.key), SuicidedKey)
	store.Set(addr.Bytes(), []byte{1})
}

func (ls *Store) GetAccountSuicided(ctx sdk.Context, addr common.Address) bool {
	store := prefix.NewStore(ctx.KVStore(ls.key), SuicidedKey)
	return store.Has(addr.Bytes())
}

func (ls *Store) GetAllSuicided(ctx sdk.Context) []common.Address {
	store := prefix.NewStore(ctx.KVStore(ls.key), SuicidedKey)

	var addrs []common.Address

	iter := sdk.KVStorePrefixIterator(store, []byte{})
	defer iter.Close()

	for ; iter.Valid(); iter.Next() {
		addr := common.BytesToAddress(iter.Key())
		addrs = append(addrs, addr)
	}

	return addrs
}
