package v2

import (
	"github.com/ethereum/go-ethereum/common"
	"log"
	"math/big"
	"testing"
)

func TestEthChainRelayer_IsW3qHeaderExistAtLightClient(t *testing.T) {
	//ethRelayer, err := NewEthChainRelayer(context.Background(), "/Users/chenyanlong/Work/go-ethereum/cmd/geth/data_val_0/keystore/UTC--2022-06-07T04-15-42.696295000Z--96f22a48dcd4dfb99a11560b24bee02f374ca77d", "123", EthereumChainConf)
	//if err != nil {
	//	panic(err)
	//}

	w3qRelayer := GlobalCoordinator.relayers[5].(*EthChainRelayer)
	ethRelayer := GlobalCoordinator.relayers[5].(*EthChainRelayer)
	// todo : check big.Int or uint64 is valid
	// both wsClient and http client validity
	res, err := ethRelayer.IsW3qHeaderExistAtLightClient(big.NewInt(0))
	if err != nil {
		t.Fatal(err)
	}

	height, err := ethRelayer.getNextEpochHeader()
	if err != nil {
		t.Fatal(err)
	}

	header, err := w3qRelayer.GetSpecificHeader(height.Uint64())
	submitTask := GlobalCoordinator.taskManager.GenSubmitWeb3qHeader_SubmitTxTask_OnEth()

	tx, err := submitTask.submitTxFunc(w3qRelayer, ethRelayer, header, submitTask)
	if err != nil {
		t.Fatal(err)
	}

	log.Println(tx.Hash())

	log.Println(ethRelayer.relayerAddr)

	log.Println(height.Uint64())

	log.Println(res)
}

func TestEthChainRelayer_GenTx(t *testing.T) {

	submitTask := GlobalCoordinator.taskManager.GenReceiveToken_SubmitTxTask_OnWeb3q()
	w3qRelayer := GlobalCoordinator.relayers[3333].(*EthChainRelayer)
	tx, err := w3qRelayer.GenTx(submitTask, common.HexToHash("0x9f031fef0bc3f78fd28a0d37d61e16ffd72487a10b82703c2ceb825116b3dc0a"), big.NewInt(0))
	if err != nil {
		t.Error(err)
	}

	err = w3qRelayer.SubmitTx(tx)
	if err != nil {
		t.Error("txHash:", tx.Hash(), "  err:", err)
	}

}
