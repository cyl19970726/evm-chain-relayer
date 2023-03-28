package v2

import (
	"log"
	"math/big"
	"testing"
)

func TestEthChainRelayer_IsW3qHeaderExistAtLightClient(t *testing.T) {
	//ethRelayer, err := NewEthChainRelayer(context.Background(), "/Users/chenyanlong/Work/go-ethereum/cmd/geth/data_val_0/keystore/UTC--2022-06-07T04-15-42.696295000Z--96f22a48dcd4dfb99a11560b24bee02f374ca77d", "123", EthereumChainConf)
	//if err != nil {
	//	panic(err)
	//}

	ethRelayer := GlobalCoordinator.relayers[5].(*EthChainRelayer)
	// todo : check big.Int or uint64 is valid
	// both wsClient and http client validity
	res, err := ethRelayer.IsW3qHeaderExistAtLightClient(big.NewInt(0))
	if err != nil {
		t.Fatal(err)
	}

	log.Println(res)
}
