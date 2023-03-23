package v2

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"errors"
	"fmt"
	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/keystore"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethdb/leveldb"
	"github.com/ethereum/go-ethereum/event"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/rlp"
	web3qtypes "github.com/web3q/core/types"
	web3qethclient "github.com/web3q/ethclient"
	"io/ioutil"
	"math/big"
	"sync/atomic"
)

type Web3qChainRelayer struct {
	ChainConfig *ChainConfig
	chainClient IChainClient
	prikey      *ecdsa.PrivateKey
	relayerAddr common.Address

	chainHeadCh     chan *web3qtypes.Header
	chainHeadSub    event.Subscription
	latestHeaderNum *big.Int

	relayerdb *leveldb.Database

	recExecTaskCh    chan Task
	recMonitorTaskCh chan *MonitorTask
	//mtQueue          []MonitorTask

	status uint32
	ctx    context.Context
	cancel context.CancelFunc
}

func NewWeb3qChainRelayer(pctx context.Context, filepath string, passwd string, conf *ChainConfig) (*Web3qChainRelayer, error) {

	ctx, cf := context.WithCancel(pctx)

	b, err := ioutil.ReadFile(filepath)
	if err != nil {
		return nil, err
	}
	key, err := keystore.DecryptKey(b, passwd)
	if err != nil {
		return nil, err
	}
	relayerAddr := crypto.PubkeyToAddress(key.PrivateKey.PublicKey)

	chainClient, err := NewW3qChainClient(conf.httpRpc, conf.wssRpc, ctx)
	if err != nil {
		return nil, err
	}

	database, err := leveldb.New("./multichain-relayers-db", 16, 16, fmt.Sprintf("./relayer-%d", conf.chainId), false)
	if err != nil {
		return nil, err
	}

	relayer := &Web3qChainRelayer{
		relayerdb:        database,
		prikey:           key.PrivateKey,
		relayerAddr:      relayerAddr,
		ChainConfig:      conf,
		chainClient:      chainClient,
		ctx:              ctx,
		cancel:           cf,
		status:           0,
		recMonitorTaskCh: make(chan *MonitorTask),
		recExecTaskCh:    make(chan Task),
	}

	sub, receiveHeaderChan, err := relayer.SubscribeLatestHeader()
	if err != nil {
		return nil, err
	}
	relayer.chainHeadSub = sub
	relayer.chainHeadCh = receiveHeaderChan
	return relayer, nil
}

func (c *Web3qChainRelayer) wsClient() *web3qethclient.Client {
	return c.chainClient.Web3qWsClient()
}

func (c *Web3qChainRelayer) httpClient() *web3qethclient.Client {
	return c.chainClient.Web3qHttpClient()
}

func (c *Web3qChainRelayer) SubscribeLatestHeader() (event.Subscription, chan *web3qtypes.Header, error) {
	var chainHeadCh = make(chan *web3qtypes.Header)
	sub, err := c.wsClient().SubscribeNewHead(c.ctx, chainHeadCh)
	if err != nil {
		return nil, nil, err
	}
	return sub, chainHeadCh, nil
}

func (c *Web3qChainRelayer) GetSpecificHeader(number uint64) (*types.Header, error) {
	return c.wsClient().HeaderByNumber(c.ctx, big.NewInt(0).SetUint64(number))
}

func (c *Web3qChainRelayer) SubscribeEvent(contract common.Address, eventId common.Hash, receiveChan chan types.Log) (event.Subscription, error) {
	sub, err := c.wsClient().SubscribeFilterLogs(c.ctx, ethereum.FilterQuery{Addresses: []common.Address{contract}, Topics: [][]common.Hash{{eventId}}}, receiveChan)
	if err != nil {
		return nil, err
	}
	return sub, nil
}

func (c *Web3qChainRelayer) signTx(tx *types.Transaction) (*types.Transaction, error) {
	signer := types.LatestSignerForChainID(big.NewInt(0).SetUint64(c.ChainId()))
	signedTx, err := types.SignTx(tx, signer, c.prikey)
	if err != nil {
		return nil, err
	}

	return signedTx, nil
}

func (c *Web3qChainRelayer) suggestGasPrice() (gasTipCap *big.Int, gasFeeCap *big.Int, err error) {
	//Estimate gasTipCap

	gasTipCap, err = c.wsClient().SuggestGasTipCap(c.ctx)
	if err != nil {
		return nil, nil, err
	}

	latestHeader, err := c.wsClient().HeaderByNumber(c.ctx, nil)
	if err != nil {
		return nil, nil, err
	}

	gasFeeCap = big.NewInt(0)
	gasFeeCap = gasFeeCap.Mul(latestHeader.BaseFee, big.NewInt(2))
	gasFeeCap = gasFeeCap.Add(gasFeeCap, gasTipCap)

	return gasTipCap, gasFeeCap, nil
}

func (c *Web3qChainRelayer) getNonce() (uint64, error) {
	relayerAddr := crypto.PubkeyToAddress(c.prikey.PublicKey)
	nonce, err := c.wsClient().PendingNonceAt(c.ctx, relayerAddr)
	return nonce, err
}

func (c *Web3qChainRelayer) estimateGas(tx *types.DynamicFeeTx) (uint64, error) {
	msg := ethereum.CallMsg{
		From:      c.relayerAddr,
		To:        tx.To,
		GasTipCap: tx.GasTipCap,
		GasFeeCap: tx.GasFeeCap,
		Value:     tx.Value,
		Data:      tx.Data,
	}
	gasLimit, err := c.wsClient().EstimateGas(c.ctx, msg)
	if err != nil {
		return 0, err
	}

	return gasLimit, nil
}

func (c *Web3qChainRelayer) GenTx(task *ExecTask, args ...interface{}) (*types.Transaction, error) {
	abi := GlobalContractInfo.GetContractAbi(c.ChainId(), task.contractAddr)
	txdata, err := abi.Pack(task.methodName, args)
	if err != nil {
		return nil, err
	}

	tx := &types.DynamicFeeTx{
		To:    &task.contractAddr,
		Data:  txdata,
		Value: big.NewInt(0),
	}

	gasLimit, err := c.estimateGas(tx)
	if err != nil {
		return nil, err
	}

	gasTipCap, gasFeeCap, err := c.suggestGasPrice()
	if err != nil {
		return nil, err
	}

	nonce, err := c.getNonce()
	if err != nil {
		return nil, err
	}

	tx.Gas = gasLimit
	tx.Nonce = nonce
	tx.GasTipCap = gasTipCap
	tx.GasFeeCap = gasFeeCap

	return types.NewTx(tx), nil
}

func (c *Web3qChainRelayer) SubmitTx(tx *types.Transaction) error {
	signedTx, err := c.signTx(tx)
	if err != nil {
		return err
	}

	err = c.httpClient().SendTransaction(c.ctx, signedTx)
	return err

}

func (c *Web3qChainRelayer) GetBlockHeader(number *big.Int) (*types.Header, error) {
	// todo : load blockHeader from db
	//get, err := c.relayerdb.Get(number.Bytes())
	//if get != nil {
	//	rlp.Decode()
	//}

	header, err := c.GetSpecificHeader(number.Uint64())
	return header, err
}

func (c *Web3qChainRelayer) CallContract(contractName string, methodName string, args ...interface{}) ([]byte, error) {

	contractInfo := GlobalContractsCfg.GetContract(contractName)
	if contractInfo == nil {
		return nil, fmt.Errorf("contract [%s] ABI no exist at GlobalContractsCfg", contractName)
	}
	packData, err := contractInfo.Abi.Pack(methodName, args)
	if err != nil {
		return nil, err
	}

	cpyAddr := contractInfo.Addr
	msg := ethereum.CallMsg{
		From:  c.relayerAddr,
		To:    &cpyAddr,
		Value: big.NewInt(0),
		Data:  packData,
	}

	res, err := c.httpClient().CallContract(c.ctx, msg, nil)
	if err != nil {
		return nil, err
	}
	return res, nil
}
func (c *Web3qChainRelayer) checkTaskValidity(task *MonitorTask) error {
	if task.TargetChainId() != c.ChainId() {
		log.Error("receive task with invalid chainId", "expect chainId", c.ChainId(), "actual chainId", task.TargetChainId())
		return fmt.Errorf("invalid chainId [%d]", task.TargetChainId())
	}

	if task.MonitorFunc == nil {
		return errors.New("task.MonitorFunc is empty")
	}

	if atomic.LoadUint32(&task.status) != MonitorTaskNoStart {
		return fmt.Errorf("task with invalid status [%d]", atomic.LoadUint32(&task.status))
	}

	return nil
}

func (c *Web3qChainRelayer) SendMonitorTask(task *MonitorTask) error {
	err := c.checkTaskValidity(task)
	if err != nil {
		return err
	}

	c.recMonitorTaskCh <- task
	return nil
}

func (c *Web3qChainRelayer) ChainId() uint64 {
	return c.ChainConfig.chainId
}

func (c *Web3qChainRelayer) Start() error {
	if atomic.LoadUint32(&c.status) != ChainRelayerNoStart {
		return fmt.Errorf("Web3qChainRelayer::Start() with invalid status [%d]", atomic.LoadUint32(&c.status))
	}
	atomic.StoreUint32(&c.status, ChainRelayerDoing)

	log.Info("ChainRelayer::Start() ChainRelayer start succeed", "chainId", c.ChainId())
	return c.Running()
}

func (c *Web3qChainRelayer) Running() error {
	if atomic.LoadUint32(&c.status) != ChainRelayerDoing {
		return fmt.Errorf("Web3qChainRelayer::Running() with invalid status [%d]", atomic.LoadUint32(&c.status))
	}
	for {
		select {
		case task := <-c.recMonitorTaskCh:
			log.Info("Web3qChainRelayer::Running() receive MonitorTask", "chainId", c.ChainId())
			if task.TargetChainId() != c.ChainId() {
				log.Error("Web3qChainRelayer::Running() receive task with invalid chainId", "expect chainId", c.ChainId(), "actual chainId", task.TargetChainId())
				continue
			}

			err := task.MonitorFunc(c)
			if err != nil {
				log.Error("Web3qChainRelayer::Running() failed to execute task.MonitorFunc()", "chainId", c.ChainId(), "err", err.Error())
				continue
			}
			// todo : It seems that move the task.StartMonitor() to taskManager is a better way
			go task.StartMonitor()

		case header := <-c.chainHeadCh:
			// store latest 100 header at stateDB
			log.Info("Web3qChainRelayer::Running get header from subscription", "chainId", c.ChainId(), "headerNum", header.Number.Uint64())
			buffer := new(bytes.Buffer)
			err := rlp.Encode(buffer, header)
			if err != nil {
				log.Error("Web3qChainRelayer::Running() failed to rlp-encoding header", "chainId", c.ChainId(), "headerNum", header.Number.Uint64(), "err", err.Error())
				// todo : take a better dealing mechanism for this case
				continue
			}

			// todo : it seems that should consider the case fork choice happened
			//get, err := c.relayerdb.Get(header.Number.Bytes())
			//if get != nil {
			//	continue
			//}

			err = c.relayerdb.Put(header.Number.Bytes(), buffer.Bytes())
			if err != nil {
				log.Error("ChainRelayer::Running() failed to put header into relayerDb", "chainId", c.ChainId(), "headerNum", header.Number.Uint64(), "err", err.Error())
				// todo : take a better dealing mechanism for this case
				continue
			}
			c.latestHeaderNum = header.Number

		case herr := <-c.chainHeadSub.Err():
			log.Error("Web3qChainRelayer::Running() the latest header subscription happened error", "chainId", c.ChainId(), "err", herr.Error())
			c.chainHeadSub.Unsubscribe()

			//sub, receiveHeaderChan, err := c.SubscribeLatestHeader()
			//if err != nil {
			//	log.Error("Web3qChainRelayer::Running() failed to resubscribe the latestHeader , prepare to quit the Web3qChainRelayer ", "chainId", c.ChainId(), "err", herr.Error())
			//	c.Stop()
			//}
			//c.chainHeadSub = sub
			//c.chainHeadCh = receiveHeaderChan

		case <-c.ctx.Done():
			log.Info("Web3qChainRelayer::Running() Web3qChainRelayer receive stop-signal and will be done ", "chainId", c.ChainId())
			atomic.StoreUint32(&c.status, ChainRelayerStopped)
			return nil
		}
	}
}

func (c *Web3qChainRelayer) Stop() error {
	if atomic.LoadUint32(&c.status) != ChainRelayerDoing {
		return fmt.Errorf("Web3qChainRelayer::Stop() with invalid status [%d]", atomic.LoadUint32(&c.status))
	}

	GlobalCoordinator.taskManager.StopTaskBySpecificChainId(c.ChainId())

	c.cancel()
	return nil
}
