package relayer

import (
	"context"
	"crypto/ecdsa"
	"evm-chain-relayer/utils"
	"fmt"
	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/accounts/keystore"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/log"
	"io/ioutil"
	"math/big"
	"os"
	"sync"
	"time"
)

type ChainConfig struct {
	logger  log.Logger
	chainId *big.Int
	httpUrl string
	wsUrl   string
}

type Relayer struct {
	filepath string
	prikey   *ecdsa.PrivateKey
}

// Relayer要实现另外一种方法从keyStore文件中直接获取私钥
func NewRelayerByKeyStore(filepath string, passwd string) (*Relayer, error) {
	b, err := ioutil.ReadFile(filepath)
	if err != nil {
		return nil, err
	}
	key, err := keystore.DecryptKey(b, passwd)
	if err != nil {
		return nil, err
	}
	return &Relayer{filepath: filepath, prikey: key.PrivateKey}, nil
}

func NewRelayer(filepath string) (*Relayer, error) {
	key, err := utils.LoadPriKey(filepath)
	if err != nil {
		return nil, err
	}
	return &Relayer{filepath: filepath, prikey: key}, nil
}

func NewChainConfig(httpUrl string, wsUrl string, logLevel int) *ChainConfig {
	glogger := log.NewGlogHandler(log.StreamHandler(os.Stderr, log.TerminalFormat(false)))
	glogger.Verbosity(log.Lvl(logLevel))
	log.Root().SetHandler(glogger)
	return &ChainConfig{logger: log.Root(), httpUrl: httpUrl, wsUrl: wsUrl}
}

func (this *ChainConfig) ChainId() *big.Int {
	return this.chainId
}

func (this *ChainConfig) setChainId(cid *big.Int) {
	this.chainId = cid
}

type ChainOperator struct {
	config         *ChainConfig
	relayer        *Relayer
	clientListener *ethclient.Client
	clientExecutor *ethclient.Client

	contracts map[common.Address]*Contract

	Ctx             context.Context
	CancelFunc      context.CancelFunc
	ListenTaskList  []*HandleEventTask
	LogsReceiveChan chan types.Log
	errChan         chan error
}

func (this *ChainOperator) name() string {
	return "ChainOperator"
}

func NewChainOperator(config *ChainConfig, r *Relayer, pctx context.Context) *ChainOperator {
	clientL, err := ethclient.Dial(config.wsUrl)
	if err != nil {
		log.Error("ethclient.Dial failed:", "err", err)
		return nil
	}

	clientE, err := ethclient.Dial(config.httpUrl)
	if err != nil {
		log.Error("ethclient.Dial failed:", err)
		return nil
	}

	ctx, cancelFunc := context.WithCancel(pctx)

	chainId, err := clientL.ChainID(ctx)
	if err != nil {
		log.Error("Get ChainId Fail:", err)
		return nil
	}
	config.setChainId(chainId)

	cchan := make(chan types.Log)

	return &ChainOperator{config: config, relayer: r, clientListener: clientL, clientExecutor: clientE, Ctx: ctx, CancelFunc: cancelFunc, LogsReceiveChan: cchan}
}

func (this *ChainOperator) Quit() {
	this.CancelFunc()
}

func (this *ChainOperator) BlockNumber() (uint64, error) {
	return this.clientExecutor.BlockNumber(this.Ctx)
}

// 处理listen逻辑：
// listener 需要达成的目标：
// 1.监听A链的合约的事件

// 需要实现的函数
// 1.注册合约 , contractAbi + address
// 2.注册要监听的合约的某一个具体事(也可以选择监听所有事件，在根据监听到的事件名字分配不同的管道)
// 3.开启监听
func (co *ChainOperator) RegisterContract(address common.Address, abi abi.ABI) {
	if co.contracts == nil {
		co.contracts = make(map[common.Address]*Contract)
	}
	co.contracts[address] = NewContract(address, abi)
}

func (co *ChainOperator) SubscribeEvent(address common.Address, eventName string, handleFunc func(types.Log)) (int, error) {
	receiveChan := make(chan types.Log)
	sCtx, cf := context.WithCancel(co.Ctx)
	sub, err := co.clientListener.SubscribeFilterLogs(sCtx, ethereum.FilterQuery{Addresses: []common.Address{address}, Topics: [][]common.Hash{{co.contracts[address].ContractAbi.Events[eventName].ID}}}, receiveChan)
	if err != nil {
		return 0, err
	}
	task := NewHandleEventTask(address, eventName, receiveChan, sub, sCtx, cf)
	if handleFunc != nil {
		task.handleFunc = handleFunc
	}

	taskIndex := len(co.ListenTaskList)
	co.ListenTaskList = append(co.ListenTaskList, task)

	eventId := co.contracts[task.address].getEventId(task.eventName)
	if co.contracts[address].HandleEventList == nil {
		co.contracts[address].HandleEventList = make(map[common.Hash]int)
	}
	co.contracts[address].HandleEventList[eventId] = taskIndex

	return taskIndex, nil
}

func (co *ChainOperator) getEventTaskIndex(address common.Address, eventName string) int {
	taskIndex := co.contracts[address].HandleEventList[co.contracts[address].getEventId(eventName)]
	return taskIndex
}

func (co *ChainOperator) getEventTask(taskIndex int) *HandleEventTask {
	return co.ListenTaskList[taskIndex]
}

func (co *ChainOperator) UnsubscribeEvent(address common.Address, eventName string) bool {
	taskIndex := co.getEventTaskIndex(address, eventName)

	task := co.ListenTaskList[taskIndex]

	if !task.start {
		return false
	}

	task.cancleFunc()
	return true
}

func (co *ChainOperator) UnsubscribeEventByIndex(taskIndex int) bool {

	task := co.ListenTaskList[taskIndex]

	if !task.start {
		return false
	}
	// send quit message
	task.cancleFunc()
	return true
}

func (co *ChainOperator) reSubscribeEvent(task *HandleEventTask) error {
	task.sub.Unsubscribe()
	task.cancleFunc()

	sCtx, cf := context.WithCancel(co.Ctx)
	sub, err := co.clientListener.SubscribeFilterLogs(sCtx, ethereum.FilterQuery{Addresses: []common.Address{task.address}, Topics: [][]common.Hash{{co.contracts[task.address].ContractAbi.Events[task.eventName].ID}}}, task.independentReceiveChan)
	if err != nil {
		return err
	}
	task.sub = sub
	task.ctx = sCtx
	task.cancleFunc = cf

	go func() {
		task.running(co)
	}()

	return nil

}

func (co *ChainOperator) StartListening() {
	var wg sync.WaitGroup
	for _, t := range co.ListenTaskList {
		task := t
		if !task.start {
			continue
		}
		wg.Add(1)
		go func() {
			curTask := task
			curTask.running(co)
			defer wg.Done()
		}()
	}
	wg.Wait()
}

func (co *ChainOperator) callContractMethod(methodName string, to common.Address, value int64, args ...interface{}) (*types.Transaction, error) {
	abi, exist := co.contracts[to]
	if !exist {
		return nil, fmt.Errorf("the address has not registered")
	}

	input, err := abi.ContractAbi.Pack(methodName, args...)
	if err != nil {
		return nil, err
	}

	return co.signedTx(to, input, value)
}

func (co *ChainOperator) signedTx(to common.Address, txdata []byte, value int64) (*types.Transaction, error) {
	relayerAddr := crypto.PubkeyToAddress(co.relayer.prikey.PublicKey)
	nonce, err := co.clientExecutor.PendingNonceAt(co.Ctx, relayerAddr)
	if err != nil {
		return nil, err
	}

	//Estimate gasTipCap
	tipCap, err := co.clientExecutor.SuggestGasTipCap(co.Ctx)
	if err != nil {
		return nil, err
	}

	latestHeader, err := co.clientExecutor.HeaderByNumber(co.Ctx, nil)
	if err != nil {
		return nil, err
	}

	gasFeeCap := new(big.Int).Add(
		tipCap, new(big.Int).Mul(latestHeader.BaseFee, big.NewInt(2)),
	)

	//msg := ethereum.CallMsg{
	//	From:      relayerAddr,
	//	To:        &to,
	//	GasTipCap: tipCap,
	//	GasFeeCap: gasFeeCap,
	//	Value:     big.NewInt(value),
	//	Data:      txdata,
	//}
	//gasLimit, err := co.clientExecutor.EstimateGas(co.Ctx, msg)
	//if err != nil {
	//	return nil, err
	//}
	//fmt.Println(gasLimit)

	tx := types.NewTx(&types.DynamicFeeTx{
		ChainID:   co.config.ChainId(),
		Nonce:     nonce,
		To:        &to,
		Value:     big.NewInt(value),
		GasTipCap: tipCap,
		GasFeeCap: gasFeeCap,
		Gas:       1000000,
		Data:      txdata,
	})

	signer := types.LatestSignerForChainID(co.config.chainId)
	signedTx, err := types.SignTx(tx, signer, co.relayer.prikey)
	if err != nil {
		return nil, err
	}

	return signedTx, nil
}

/**
 * Contract Listener
**/
type Contract struct {
	Addr            common.Address
	ContractAbi     abi.ABI
	HandleEventList map[common.Hash]int
}

func NewContract(addr common.Address, contractAbi abi.ABI) *Contract {
	return &Contract{Addr: addr, ContractAbi: contractAbi}
}

func (c *Contract) getEventId(name string) common.Hash {
	return c.ContractAbi.Events[name].ID
}

// 监听 ethereum 的 burn 事件
//
// 在 web3q 上执行mint 事件

/*
	Handle Func
*/
func (c *ChainOperator) mintNative(to common.Address, txHash common.Hash, logIdx int64) (*types.Transaction, error) {
	tx, err := c.callContractMethod("mintNative", to, 0, txHash, big.NewInt(logIdx))
	if err != nil {
		return nil, err
	}

	return c.signedTx(to, tx.Data(), tx.Value().Int64())
}

func (c *ChainOperator) MintNative(to common.Address, ethChainOperator *ChainOperator) func(types.Log) {
	return func(log types.Log) {

		// 生成一个已经签名好的交易
		signedTx, err := c.mintNative(to, log.TxHash, int64(log.Index))
		if err != nil {
			panic(err)
		}

		err = c.NewMintNativeTask(ethChainOperator, c, log.BlockNumber).Doing(signedTx)
		if err != nil {
			c.config.logger.Error("failed to send transaction to blockchain", "ChainId", signedTx.ChainId(), "TxHash", signedTx.Hash())
			return
		}
		c.config.logger.Info("send transaction succeed!", "ChainId", signedTx.ChainId(), "TxHash", signedTx.Hash())

	}
}

func (w3q *ChainOperator) NewMintNativeTask(ethChainOperator *ChainOperator, w3qChainOperator *ChainOperator, ethTxHappened uint64) *CrossChainCallTask {
	return NewCrossChainCallTask(w3q.Ctx, ethChainOperator, w3qChainOperator, true, make(chan struct{}), ethChainOperator.waitingExpectHeight, ethTxHappened+10)
}

func (c *ChainOperator) mintW3qErc20(to common.Address, receiptHash common.Hash) {

}

type Task interface {
	Doing(transaction *types.Transaction, height uint64) error
}

// 是否需要一个在监听到某个区块之后去执行的taskPool
// 将mintNative的执行过程封装成另外一种task
// 这种task有一下特性
// 1. 它是要到另一条chain上去执行的
// 2. 它需要在本链达成额外的一些条件，在dstChain 才能执行成功
// 3. 所以它应该属于一种delpayTask
// 4. 因此我需要一些flag来标识这种task

type CrossChainCallTask struct {
	ctx               context.Context
	srcChainOperator  *ChainOperator
	dstChainOperator  *ChainOperator
	delay             bool
	delayChan         chan struct{}
	delayFunc         func(chan struct{}, uint64) error
	ExpectHeightOnETH uint64
}

func NewCrossChainCallTask(ctx context.Context, srcChainOperator *ChainOperator, dstChainOperator *ChainOperator, delay bool, delayChan chan struct{}, delayFunc func(chan struct{}, uint64) error, ethTxHappened uint64) *CrossChainCallTask {
	return &CrossChainCallTask{ctx: ctx, srcChainOperator: srcChainOperator, dstChainOperator: dstChainOperator, delay: delay, delayChan: delayChan, delayFunc: delayFunc, ExpectHeightOnETH: ethTxHappened + 10}
}

func (c *CrossChainCallTask) Doing(tx *types.Transaction) error {
	if c.delay {
		go c.delayFunc(c.delayChan, c.ExpectHeightOnETH)
		<-c.delayChan
		return c.dstChainOperator.clientExecutor.SendTransaction(c.ctx, tx)
	} else {
		return c.dstChainOperator.clientExecutor.SendTransaction(c.ctx, tx)
	}
}

func (c *ChainOperator) waitingExpectHeight(delay chan struct{}, expectHeight uint64) error {
	for {
		number, err := c.clientExecutor.BlockNumber(c.Ctx)
		if err != nil {
			return err
		}

		if number < expectHeight {
			fmt.Println("交易等待中")
			var delayNumber int64 = int64(expectHeight-number) * 15
			st := int64(delayNumber) * int64(time.Second)
			time.Sleep(time.Duration(st))
		} else {
			log.Info("发送交易条件已经达成")
			delay <- struct{}{}
			break
		}
	}
	return nil
}
