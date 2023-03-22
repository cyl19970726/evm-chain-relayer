package main

import (
	"evm-chain-relayer/v2"
	"fmt"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"time"
)

const cabi = "[\n\t{\n\t\t\"inputs\": [],\n\t\t\"name\": \"happen\",\n\t\t\"outputs\": [],\n\t\t\"stateMutability\": \"nonpayable\",\n\t\t\"type\": \"function\"\n\t},\n\t{\n\t\t\"anonymous\": false,\n\t\t\"inputs\": [\n\t\t\t{\n\t\t\t\t\"indexed\": true,\n\t\t\t\t\"internalType\": \"address\",\n\t\t\t\t\"name\": \"addr\",\n\t\t\t\t\"type\": \"address\"\n\t\t\t},\n\t\t\t{\n\t\t\t\t\"indexed\": false,\n\t\t\t\t\"internalType\": \"uint256\",\n\t\t\t\t\"name\": \"value\",\n\t\t\t\t\"type\": \"uint256\"\n\t\t\t}\n\t\t],\n\t\t\"name\": \"myemit\",\n\t\t\"type\": \"event\"\n\t},\n\t{\n\t\t\"inputs\": [],\n\t\t\"name\": \"value\",\n\t\t\"outputs\": [\n\t\t\t{\n\t\t\t\t\"internalType\": \"uint256\",\n\t\t\t\t\"name\": \"\",\n\t\t\t\t\"type\": \"uint256\"\n\t\t\t}\n\t\t],\n\t\t\"stateMutability\": \"view\",\n\t\t\"type\": \"function\"\n\t}\n]"

const rinkebyRpcUrl = "https://rinkeby.infura.io/v3/4e3e18f80d8d4ad5959b7404e85e0143"
const rinkebyRpcWsUrl = "wss://rinkeby.infura.io/ws/v3/4e3e18f80d8d4ad5959b7404e85e0143"

var w3qERC20Addr = common.HexToAddress("0xb0BC3A6071c2243C4D2E3f7303dc43fB3eE744ff")

const web3QRPCUrl = "http://127.0.0.1:8545"
const web3QValKetStoreFilePath = "/Users/chenyanlong/Work/go-ethereum/cmd/geth/data_val_0/keystore/UTC--2022-06-07T04-15-42.696295000Z--96f22a48dcd4dfb99a11560b24bee02f374ca77d"
const passwd = "123"

var w3qNativeContractAddr = common.HexToAddress("0x0000000000000000000000000000000003330002")

//func main1() {
//	// initialize rinkeby chainOperator
//	rinkebyConfig := relayer.NewChainConfig(rinkebyRpcUrl, rinkebyRpcWsUrl, 3)
//	//relayerIn := relayer.NewRelayer("")
//	rinkebyOperator := relayer.NewChainOperator(rinkebyConfig, nil, context.Background())
//	if rinkebyOperator == nil {
//		panic("exec is nil")
//	}
//	json, err := abi.JSON(strings.NewReader(relayer.W3qERC20ABI))
//	if err != nil {
//		panic(err)
//	}
//	rinkebyOperator.RegisterContract(w3qERC20Addr, json)
//
//	// initialize web3q chainOperator
//	web3qRelayer, err := relayer.NewRelayerByKeyStore(web3QValKetStoreFilePath, passwd)
//	if err != nil {
//		panic(err)
//	}
//	web3QConfig := relayer.NewChainConfig(web3QRPCUrl, web3QRPCUrl, 3)
//	web3QOperator := relayer.NewChainOperator(web3QConfig, web3qRelayer, context.Background())
//	if rinkebyOperator == nil {
//		panic("exec is nil")
//	}
//	web3qNativeJson, err := abi.JSON(strings.NewReader(relayer.W3qNativeTestABI))
//	if err != nil {
//		panic(err)
//	}
//	web3QOperator.RegisterContract(w3qNativeContractAddr, web3qNativeJson)
//
//	// rinkebyChainOperator subscribe events
//	mintTaskIndex, err := rinkebyOperator.SubscribeEvent(w3qERC20Addr, "mintToken", nil)
//	if err != nil {
//		panic(err)
//	}
//	fmt.Println("mintTaskIndex:", mintTaskIndex)
//
//	burnTaskIndex, err := rinkebyOperator.SubscribeEvent(w3qERC20Addr, "burnToken", web3QOperator.MintNative(w3qNativeContractAddr, rinkebyOperator))
//	if err != nil {
//		panic(err)
//	}
//	fmt.Println("burnTaskIndex:", burnTaskIndex)
//
//	//approvalTaskIndex, err := rinkebyOperator.SubscribeEvent(w3qERC20Addr, "Transfer", nil)
//	//if err != nil {
//	//	panic(err)
//	//}
//	//fmt.Println("burnTaskIndex:", approvalTaskIndex)
//
//	//web3qChainOperator subscribe events
//	//w3qBurnIndex, err := web3QOperator.SubscribeEvent(w3qNativeContractAddr, "burnNativeToken", nil)
//	//if err != nil {
//	//	panic(err)
//	//}
//	//fmt.Println("w3qBurnIndex:", w3qBurnIndex)
//	//w3qMintIndex, err := web3QOperator.SubscribeEvent(w3qNativeContractAddr, "mintNativeToken", nil)
//	//if err != nil {
//	//	panic(err)
//	//}
//	//fmt.Println("w3qMintIndex:", w3qMintIndex)
//
//	rinkebyOperator.StartListening()
//	//web3QOperator.StartListening()
//}

func printLog(log types.Log) {
	fmt.Println("handle log")
	fmt.Println("log topics", log.Topics[0], log.Topics[1])
	fmt.Println("log data", log.Data)
}

func main() {
	// 1. Monitor Subscription
	//
	go v2.GlobalCoordinator.Start()

	time.Sleep(500 * time.Second)
	//fmt.Println("Jumping")
	//v2.GlobalCoordinator.Stop()
}
