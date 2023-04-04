package v2

import (
	"context"
	"crypto/ecdsa"
	"errors"
	"fmt"
	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/keystore"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/ethdb/leveldb"
	"github.com/ethereum/go-ethereum/event"
	"github.com/ethereum/go-ethereum/log"
	"io/ioutil"
	"math/big"
	"sync/atomic"
)

type EthChainRelayer struct {
	ChainConfig *ChainConfig
	chainClient IChainClient
	prikey      *ecdsa.PrivateKey
	relayerAddr common.Address

	chainHeadCh     chan *types.Header
	chainHeadSub    event.Subscription
	latestHeaderNum *big.Int

	relayerdb *leveldb.Database

	recExecTaskCh    chan Task
	recMonitorTaskCh chan IMonitorTask
	//mtQueue          []MonitorTask

	status uint32
	ctx    context.Context
	cancel context.CancelFunc
}

func NewEthChainRelayer(pctx context.Context, filepath string, passwd string, conf *ChainConfig) (*EthChainRelayer, error) {

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

	chainClient, err := NewEthChainClient(conf.httpRpc, conf.wssRpc, ctx)
	if err != nil {
		return nil, err
	}

	database, err := leveldb.New(conf.leveldbDir, 16, 16, fmt.Sprintf("./relayer-%d", conf.chainId), false)
	if err != nil {
		return nil, err
	}

	relayer := &EthChainRelayer{
		relayerdb:        database,
		prikey:           key.PrivateKey,
		relayerAddr:      relayerAddr,
		ChainConfig:      conf,
		chainClient:      chainClient,
		ctx:              ctx,
		cancel:           cf,
		status:           ChainRelayerNoStart,
		recMonitorTaskCh: make(chan IMonitorTask),
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

func (c *EthChainRelayer) wsClient() *ethclient.Client {
	return c.chainClient.EthWsClient()
}

func (c *EthChainRelayer) httpClient() *ethclient.Client {
	return c.chainClient.EthHttpClient()
}

func (c *EthChainRelayer) SubscribeLatestHeader() (event.Subscription, chan *types.Header, error) {
	var chainHeadCh = make(chan *types.Header)
	sub, err := c.wsClient().SubscribeNewHead(c.ctx, chainHeadCh)
	if err != nil {
		return nil, nil, err
	}
	return sub, chainHeadCh, nil
}

func (c *EthChainRelayer) GetSpecificHeader(number uint64) (*types.Header, error) {
	return c.wsClient().HeaderByNumber(c.ctx, big.NewInt(0).SetUint64(number))
}

func (c *EthChainRelayer) SubscribeEvent(contract common.Address, eventId common.Hash, receiveChan chan types.Log) (event.Subscription, error) {
	sub, err := c.wsClient().SubscribeFilterLogs(c.ctx, ethereum.FilterQuery{Addresses: []common.Address{contract}, Topics: [][]common.Hash{{eventId}}}, receiveChan)
	if err != nil {
		return nil, err
	}
	return sub, nil
}

func (c *EthChainRelayer) signTx(tx *types.Transaction) (*types.Transaction, error) {
	signer := types.LatestSignerForChainID(big.NewInt(0).SetUint64(c.ChainId()))
	signedTx, err := types.SignTx(tx, signer, c.prikey)
	if err != nil {
		return nil, err
	}

	return signedTx, nil
}

func (c *EthChainRelayer) suggestGasPrice() (gasTipCap *big.Int, gasFeeCap *big.Int, err error) {
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

func (c *EthChainRelayer) getNonce() (uint64, error) {
	relayerAddr := crypto.PubkeyToAddress(c.prikey.PublicKey)
	nonce, err := c.wsClient().PendingNonceAt(c.ctx, relayerAddr)
	return nonce, err
}

func (c *EthChainRelayer) estimateGas(tx *types.DynamicFeeTx) (uint64, error) {
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

func (c *EthChainRelayer) GenTx(task *SubmitTxTask, args ...interface{}) (*types.Transaction, error) {
	abi := GlobalContractsCfg.GetContractAbi(task.contractName)
	txdata, err := abi.Pack(task.methodName, args...)
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

func (c *EthChainRelayer) GenTx1(task *SubmitTxTask, args ...interface{}) (*types.Transaction, error) {
	abi := GlobalContractsCfg.GetContractAbi(task.contractName)
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

func (c *EthChainRelayer) SubmitTx(tx *types.Transaction) error {
	signedTx, err := c.signTx(tx)
	if err != nil {
		return err
	}

	err = c.httpClient().SendTransaction(c.ctx, signedTx)
	return err

}

func (c *EthChainRelayer) GetBlockHeader(number *big.Int) (*types.Header, error) {
	// todo : load blockHeader from db
	//get, err := c.relayerdb.Get(number.Bytes())
	//if get != nil {
	//	rlp.Decode()
	//}

	header, err := c.GetSpecificHeader(number.Uint64())
	return header, err
}

func (c *EthChainRelayer) GetReceiptProof(txhash common.Hash) (*ethclient.ReceiptProofData, error) {
	proof, err := c.wsClient().ReceiptProof(c.ctx, txhash)
	return proof, err
}

func (c *EthChainRelayer) CallContract(contractName string, methodName string, args ...interface{}) ([]byte, error) {

	contractInfo := GlobalContractsCfg.GetContract(contractName)
	if contractInfo == nil {
		return nil, fmt.Errorf("contract [%s] ABI no exist at GlobalContractsCfg", contractName)
	}
	packData, err := contractInfo.Abi.Pack(methodName, args...)
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

	res, err := c.wsClient().CallContract(c.ctx, msg, nil)
	if err != nil {
		return nil, err
	}
	return res, nil
}

func (c *EthChainRelayer) getNextEpochHeader() (*big.Int, error) {
	res, err := c.CallContract(LightClientContract, GetNextEpochHeightFunc)
	if err != nil {
		return nil, err
	}

	height := big.NewInt(0).SetBytes(res)
	return height, nil
}

func (c *EthChainRelayer) IsW3qHeaderExistAtLightClient(web3qHeadrNumber *big.Int) (bool, error) {

	res, err := c.CallContract(LightClientContract, BlockExistFunc, web3qHeadrNumber)
	if err != nil {
		return false, err
	}

	exist := big.NewInt(0).SetBytes(res)
	if exist.Uint64() == 0 {
		return false, nil
	} else {
		return true, nil
	}
}

func (c *EthChainRelayer) checkTaskValidity(task IMonitorTask) error {
	if task.TargetChainId() != c.ChainId() {
		log.Error("receive task with invalid chainId", "expect chainId", c.ChainId(), "actual chainId", task.TargetChainId())
		return fmt.Errorf("invalid chainId [%d]", task.TargetChainId())
	}

	if task.GetMonitorFunc() == nil {
		return errors.New("task.MonitorFunc is empty")
	}

	if task.Status() != MonitorTaskNoStart {
		return fmt.Errorf("task with invalid status [%d]", task.Status())
	}

	return nil
}

func (c *EthChainRelayer) SendMonitorTask(task IMonitorTask) error {
	err := c.checkTaskValidity(task)
	if err != nil {
		return err
	}

	c.recMonitorTaskCh <- task
	return nil
}

func (c *EthChainRelayer) ChainId() uint64 {
	return c.ChainConfig.chainId
}

func (c *EthChainRelayer) Start() error {
	if atomic.LoadUint32(&c.status) != ChainRelayerNoStart {
		return fmt.Errorf("EthChainRelayer::Start() with invalid status [%d]", atomic.LoadUint32(&c.status))
	}
	atomic.StoreUint32(&c.status, ChainRelayerDoing)

	log.Info("ChainRelayer::Start() ChainRelayer start succeed", "chainId", c.ChainId())
	return c.running()
}

func (c *EthChainRelayer) running() error {
	if atomic.LoadUint32(&c.status) != ChainRelayerDoing {
		return fmt.Errorf("EthChainRelayer::Running() with invalid status [%d]", atomic.LoadUint32(&c.status))
	}
	for {
		select {
		case task := <-c.recMonitorTaskCh:
			log.Info("EthChainRelayer::Running() receive MonitorTask", "chainId", c.ChainId())
			if task.TargetChainId() != c.ChainId() {
				log.Error("EthChainRelayer::Running() receive task with invalid chainId", "expect chainId", c.ChainId(), "actual chainId", task.TargetChainId())
				continue
			}

			err := task.ExecMonitorFunc(c)
			if err != nil {
				log.Error("EthChainRelayer::Running() failed to execute task.MonitorFunc()", "chainId", c.ChainId(), "err", err.Error())
				continue
			}
			// todo : It seems that move the task.StartMonitor() to taskManager is a better way
			go task.StartMonitor()

		//case header := <-c.chainHeadCh:
		//	// store latest 100 header at stateDB
		//	log.Info("EthChainRelayer::running() get header from subscription", "chainId", c.ChainId(), "headerNum", header.Number.Uint64())
		//	buffer := new(bytes.Buffer)
		//	err := rlp.Encode(buffer, header)
		//	if err != nil {
		//		log.Error("EthChainRelayer::running() failed to rlp-encoding header", "chainId", c.ChainId(), "headerNum", header.Number.Uint64(), "err", err.Error())
		//		// todo : take a better dealing mechanism for this case
		//		continue
		//	}
		//
		//	// todo : it seems that should consider the case fork choice happened
		//	//get, err := c.relayerdb.Get(header.Number.Bytes())
		//	//if get != nil {
		//	//	continue
		//	//}
		//
		//	err = c.relayerdb.Put(header.Number.Bytes(), buffer.Bytes())
		//	if err != nil {
		//		log.Error("ChainRelayer::Running() failed to put header into relayerDb", "chainId", c.ChainId(), "headerNum", header.Number.Uint64(), "err", err.Error())
		//		// todo : take a better dealing mechanism for this case
		//		continue
		//	}
		//	c.latestHeaderNum = header.Number
		//
		//case herr := <-c.chainHeadSub.Err():
		//	log.Error("EthChainRelayer::Running() the latest header subscription happened error", "chainId", c.ChainId(), "err", herr.Error())
		//	c.chainHeadSub.Unsubscribe()
		//
		//	sub, receiveHeaderChan, err := c.SubscribeLatestHeader()
		//	if err != nil {
		//		log.Error("EthChainRelayer::Running() failed to resubscribe the latestHeader , prepare to quit the EthChainRelayer ", "chainId", c.ChainId(), "err", herr.Error())
		//		c.Stop()
		//	}
		//	c.chainHeadSub = sub
		//	c.chainHeadCh = receiveHeaderChan

		case <-c.ctx.Done():
			log.Info("EthChainRelayer::Running() EthChainRelayer receive stop-signal and will be done ", "chainId", c.ChainId())
			atomic.StoreUint32(&c.status, ChainRelayerStopped)
			return nil
		}
	}
}

func (c *EthChainRelayer) Stop() error {
	if atomic.LoadUint32(&c.status) != ChainRelayerDoing {
		return fmt.Errorf("EthChainRelayer::Stop() with invalid status [%d]", atomic.LoadUint32(&c.status))
	}

	GlobalCoordinator.taskManager.StopTaskBySpecificChainId(c.ChainId())

	c.cancel()
	return nil
}
