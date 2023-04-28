package keeper_test

import (
	"reflect"

	"github.com/cosmos/cosmos-sdk/testutil"
	sdk "github.com/cosmos/cosmos-sdk/types"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	paramtypes "github.com/cosmos/cosmos-sdk/x/params/types"
	"github.com/evmos/ethermint/app"
	"github.com/evmos/ethermint/encoding"
	"github.com/evmos/ethermint/x/evm/keeper"
	"github.com/evmos/ethermint/x/evm/types"
	"github.com/evmos/ethermint/x/evm/vm/geth"
)

func (suite *KeeperTestSuite) TestParams() {
	params := suite.app.EvmKeeper.GetParams(suite.ctx)
	suite.app.EvmKeeper.SetParams(suite.ctx, params)
	testCases := []struct {
		name      string
		paramsFun func() interface{}
		getFun    func() interface{}
		expected  bool
	}{
		{
			"success - Checks if the default params are set correctly",
			func() interface{} {
				return types.DefaultParams()
			},
			func() interface{} {
				return suite.app.EvmKeeper.GetParams(suite.ctx)
			},
			true,
		},
		{
			"success - EvmDenom param is set to \"inj\" and can be retrieved correctly",
			func() interface{} {
				params.EvmDenom = "inj"
				suite.app.EvmKeeper.SetParams(suite.ctx, params)
				return params.EvmDenom
			},
			func() interface{} {
				evmParams := suite.app.EvmKeeper.GetParams(suite.ctx)
				return evmParams.GetEvmDenom()
			},
			true,
		},
		{
			"success - Check EnableCreate param is set to false and can be retrieved correctly",
			func() interface{} {
				params.EnableCreate = false
				suite.app.EvmKeeper.SetParams(suite.ctx, params)
				return params.EnableCreate
			},
			func() interface{} {
				evmParams := suite.app.EvmKeeper.GetParams(suite.ctx)
				return evmParams.GetEnableCreate()
			},
			true,
		},
		{
			"success - Check EnableCall param is set to false and can be retrieved correctly",
			func() interface{} {
				params.EnableCall = false
				suite.app.EvmKeeper.SetParams(suite.ctx, params)
				return params.EnableCall
			},
			func() interface{} {
				evmParams := suite.app.EvmKeeper.GetParams(suite.ctx)
				return evmParams.GetEnableCall()
			},
			true,
		},
		{
			"success - Check AllowUnprotectedTxs param is set to false and can be retrieved correctly",
			func() interface{} {
				params.AllowUnprotectedTxs = false
				suite.app.EvmKeeper.SetParams(suite.ctx, params)
				return params.AllowUnprotectedTxs
			},
			func() interface{} {
				evmParams := suite.app.EvmKeeper.GetParams(suite.ctx)
				return evmParams.GetAllowUnprotectedTxs()
			},
			true,
		},
		{
			"success - Check ChainConfig param is set to the default value and can be retrieved correctly",
			func() interface{} {
				params.ChainConfig = types.DefaultChainConfig()
				suite.app.EvmKeeper.SetParams(suite.ctx, params)
				return params.ChainConfig
			},
			func() interface{} {
				evmParams := suite.app.EvmKeeper.GetParams(suite.ctx)
				return evmParams.GetChainConfig()
			},
			true,
		},
	}
	for _, tc := range testCases {
		suite.Run(tc.name, func() {
			outcome := reflect.DeepEqual(tc.paramsFun(), tc.getFun())
			suite.Require().Equal(tc.expected, outcome)
		})
	}
}

func (suite *KeeperTestSuite) TestLegacyParamsKeyTableRegistration() {
	encCfg := encoding.MakeConfig(app.ModuleBasics)
	cdc := encCfg.Codec
	storeKey := sdk.NewKVStoreKey(types.ModuleName)
	tKey := sdk.NewTransientStoreKey(types.TransientKey)
	ctx := testutil.DefaultContext(storeKey, tKey)
	ak := suite.app.AccountKeeper

	// paramspace used only for setting legacy parameters (not given to keeper)
	setParamSpace := paramtypes.NewSubspace(
		cdc,
		encCfg.Amino,
		storeKey,
		tKey,
		"evm",
	).WithKeyTable(types.ParamKeyTable())
	params := types.DefaultParams()
	setParamSpace.SetParamSet(ctx, &params)

	// param space that has not been created with a key table
	unregisteredSubspace := paramtypes.NewSubspace(
		cdc,
		encCfg.Amino,
		storeKey,
		tKey,
		"evm",
	)

	// assertion required to ensure we are testing correctness
	// of a keeper receiving a subpsace without a key table registration
	suite.Require().False(unregisteredSubspace.HasKeyTable())

	newKeeper := func() *keeper.Keeper {
		// create a keeper, mimicking an app.go which has not registered the key table
		return keeper.NewKeeper(
			cdc, storeKey, tKey, authtypes.NewModuleAddress("gov"),
			ak,
			nil, nil, nil, nil, // OK to pass nil in for these since we only instantiate and use params
			geth.NewEVM,
			"",
			unregisteredSubspace,
		)
	}
	k := newKeeper()

	// the keeper must set the key table
	var fetchedParams types.Params
	suite.Require().NotPanics(func() { fetchedParams = k.GetParams(ctx) })
	// this modifies the internal data of the subspace, so we should see the key table registered
	suite.Require().True(unregisteredSubspace.HasKeyTable())
	// general check that params match what we set and are not nil
	suite.Require().Equal(params, fetchedParams)
	// ensure we do not attempt to override any existing key tables to keep compatibility
	// when passing a subpsace to the keeper that has already been used to work with parameters
	suite.Require().NotPanics(func() { newKeeper() })
}
