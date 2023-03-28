package v2

import (
	"github.com/ethereum/go-ethereum/common"
)

var (
	Web3qChainConf = &ChainConfig{
		chainId:    3333,
		httpRpc:    "",
		wssRpc:     "",
		bridgeAddr: common.HexToAddress("0x0000000000000000000000000000000003330002"),
	}

	EthereumChainConf = &ChainConfig{
		chainId:         5,
		httpRpc:         "https://goerli.infura.io/v3/91ee2301e9004952b8cecaae307b1c28",
		wssRpc:          "wss://goerli.infura.io/ws/v3/91ee2301e9004952b8cecaae307b1c28",
		bridgeAddr:      common.HexToAddress("0x145dEA5dE0c4F626028220De757430b4e5E7Db7D"),
		lightClientAddr: common.HexToAddress("0xbc21ef51B936EDf9e9E5b8F4D2be5AcF58ab4C99"),
	}
)

type ChainConfig struct {
	chainId         uint64
	httpRpc         string
	wssRpc          string
	bridgeAddr      common.Address
	lightClientAddr common.Address
}

func NewChainConfig(httpUrl string, wsUrl string, logLevel int) *ChainConfig {
	return &ChainConfig{httpRpc: httpUrl, wssRpc: wsUrl}
}
