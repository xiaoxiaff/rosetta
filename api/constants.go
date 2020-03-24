package api

import (
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/params"
)

// TODO(yorke): consider reading version info from disk
const (
	RosettaVersion    = "1.2.4"
	MiddlewareVersion = "1.0.0"
	BlockchainName    = "celo"
)

var (
	NodeVersion      = params.Version
	ChainIdToNetwork = map[uint64]string{
		44786:  "alfajores",
		200110: "baklava",
		200312: "rc0",
	}
	RegistrySmartContractAddress = common.HexToAddress("0x000000000000000000000000000000000000ce10")
	LockedGoldRegistryId         = "LockedGold"
	StableTokenRegistryId        = "StableToken"
	CeloGold                     = Currency{
		Symbol:   "cGLD",
		Decimals: 18,
	}
	CeloDollar = Currency{
		Symbol:   "cUSD",
		Decimals: 18,
	}
)
