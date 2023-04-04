package v2

import (
	"github.com/ethereum/go-ethereum/common"
)

var (
	Web3qChainConf = &ChainConfig{
		chainId:    3333,
		httpRpc:    "http://127.0.0.1:8545",
		wssRpc:     "ws://127.0.0.1:8546",
		bridgeAddr: common.HexToAddress("0x0000000000000000000000000000000003330002"),
		leveldbDir: "./ldb-w3q",
	}

	EthereumChainConf = &ChainConfig{
		chainId:         5,
		httpRpc:         "https://goerli.infura.io/v3/91ee2301e9004952b8cecaae307b1c28",
		wssRpc:          "wss://goerli.infura.io/ws/v3/91ee2301e9004952b8cecaae307b1c28",
		bridgeAddr:      common.HexToAddress("0x0C31d8aCF362353622F16F24A576a310A75312FA"),
		lightClientAddr: common.HexToAddress("0xCb101a3fEe489E8ef3E713F8085d241849bf8382"),
		leveldbDir:      "./ldb-eth",
	}
)

type ChainConfig struct {
	chainId         uint64
	httpRpc         string
	wssRpc          string
	bridgeAddr      common.Address
	lightClientAddr common.Address
	leveldbDir      string
}

func NewChainConfig(httpUrl string, wsUrl string, logLevel int) *ChainConfig {
	return &ChainConfig{httpRpc: httpUrl, wssRpc: wsUrl}
}
