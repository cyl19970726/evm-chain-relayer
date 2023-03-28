package v2

import (
	"fmt"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/rlp"
	"sync"
	"sync/atomic"
	"time"
)

const (
	SubmitTxTaskNoStart  = 0
	SubmitTxTaskDoing    = 1
	SubmitTxTaskStopping = 2
	SubmitTxTaskStopped  = 3
)

type (
	SubmitTxTask struct {
		targetChainId uint64
		contractAddr  common.Address
		contractName  string
		methodName    string

		status uint32

		submitTxFunc func(c *EthChainRelayer, value interface{}, task *SubmitTxTask) error

		pwg       sync.WaitGroup
		receiveCh chan interface{}
		cancelCh  chan struct{}
	}
)

func NewSubmitTxTask(caddr common.Address, cName, mName string, targetChainId uint64, pwg sync.WaitGroup) *SubmitTxTask {
	return &SubmitTxTask{
		contractAddr:  caddr,
		contractName:  cName,
		methodName:    mName,
		status:        SubmitTxTaskNoStart,
		targetChainId: targetChainId,
		receiveCh:     make(chan interface{}),
		cancelCh:      make(chan struct{}),
		pwg:           pwg,
	}
}

func (et *SubmitTxTask) Name() string {
	return "SubmitTxTask"
}

func (et *SubmitTxTask) Start() error {
	et.pwg.Add(1)
	defer et.pwg.Done()
	if et.Status() != SubmitTxTaskNoStart {
		return fmt.Errorf("SubmitTxTask::Start() already started")
	}

	et.SetStatus(SubmitTxTaskDoing)

	return et.running()
}
func (et *SubmitTxTask) running() error {
	for {
		select {
		case data := <-et.receiveCh:
			//err := et.execFunc(data)
			r := GlobalCoordinator.GetRelayer(et.targetChainId).(*EthChainRelayer)
			if r == nil {
				return fmt.Errorf("chainRelayer %d no exist", et.targetChainId)
			}
			err := et.submitTxFunc(r, data, et)
			// todo : how to deal failed tx
			log.Error("SubmitTxTask::running() failed to submitTx", "chainId", et.TargetChainId(), "methodName", et.methodName, "error", err.Error())
		case <-et.cancelCh:
			et.SetStatus(SubmitTxTaskStopped)
			return nil
		}
	}
}

func (task *SubmitTxTask) TargetChainId() uint64 {
	return task.targetChainId
}

func (task *SubmitTxTask) Stop() error {
	log.Info("MonitorTask::Stop() send the stop-signal to the monitor-contract-event task ", "chainId", task.TargetChainId(), "contract", task.contractAddr)
	if task.Status() == SubmitTxTaskDoing {
		task.cancelCh <- struct{}{}
		task.SetStatus(SubmitTxTaskStopping)
		return nil
	}

	return fmt.Errorf("MonitorTask::Stop() failed to stop the monitor-contract-event task execution")

}

func (task *SubmitTxTask) Status() uint32 {
	return atomic.LoadUint32(&task.status)
}

func (task *SubmitTxTask) SetStatus(newStatus uint32) {
	atomic.StoreUint32(&task.status, newStatus)
}

func (manager *TaskManager) GenReceiveToken_SubmitTxTask_OnEth() *SubmitTxTask {
	task := NewSubmitTxTask(EthereumChainConf.bridgeAddr, LightClientContract, receiveFromWeb3qFunc, EthereumChainConf.chainId, manager.wg)
	ef := func(c *EthChainRelayer, value interface{}, task *SubmitTxTask) error {
		log := value.(*types.Log)
		// 4. get receipt_proof from web3q
		proof, err := c.GetReceiptProof(log.TxHash)
		if err != nil {
			return err
		}

		// todo : check big.Int or uint63 is valid
		tx, err := c.GenTx(task, log.BlockNumber, proof, log.Index)
		if err != nil {
			return err
		}
		err = c.SubmitTx(tx)
		if err != nil {
			return err
		}

		return nil
	}

	task.submitTxFunc = ef
	return task
}

func (manager *TaskManager) GenSubmitWeb3qHeader_SubmitTxTask_OnEth() *SubmitTxTask {
	task := NewSubmitTxTask(EthereumChainConf.lightClientAddr, LightClientContract, receiveFromWeb3qFunc, EthereumChainConf.chainId, manager.wg)
	ef := func(c *EthChainRelayer, value interface{}, task *SubmitTxTask) error {
		web3qHeader := value.(*types.Header)

		//3. submit Header to Ethereum
		eHeader, eCommit, err := PackedWeb3qHeader(web3qHeader)
		if err != nil {
			return err
		}

		// todo: getNonce

		tx, err := c.GenTx(task, web3qHeader.Number, eHeader, eCommit, false)
		if err != nil {
			return err
		}

		err = c.SubmitTx(tx)
		if err != nil {
			return err
		}

		// todo : set the current txHash for exec_task , and track the tx

		return nil
	}

	task.submitTxFunc = ef
	return task
}

const CONFIRMS = 10
const BlockInternalSecond = 10

func (manager *TaskManager) GenReceiveToken_SubmitTxTask_OnWeb3q() *SubmitTxTask {
	task := NewSubmitTxTask(Web3qChainConf.bridgeAddr, Web3qBridgeContract, receiveFromWeb3qFunc, Web3qChainConf.chainId, manager.wg)
	ef := func(c *EthChainRelayer, value interface{}, task *SubmitTxTask) error {
		log := value.(*types.Log)

		expectBN := log.BlockNumber + CONFIRMS
		for {
			latestBN := c.latestHeaderNum.Uint64()
			if latestBN > expectBN {
				tx, err := c.GenTx(task, log.TxHash, log.Index)
				if err != nil {
					return err
				}
				err = c.SubmitTx(tx)
				return nil
			}

			var ss int64 = int64((expectBN - latestBN) * BlockInternalSecond)
			var totalTime int64 = ss * time.Second.Nanoseconds()
			time.Sleep(time.Duration(totalTime))
		}

	}
	task.submitTxFunc = ef
	return task
}

func PackedWeb3qHeader(header *types.Header) ([]byte, []byte, error) {
	cph := types.CopyHeader(header)
	cph.Commit = nil
	eHeader, err := rlp.EncodeToBytes(cph)
	if err != nil {
		return nil, nil, err
	}

	cph = types.CopyHeader(header)
	cph.Commit = nil
	eHeader, err = rlp.EncodeToBytes(cph)
	if err != nil {
		return nil, nil, err
	}
	eCommit, err := rlp.EncodeToBytes(header.Commit)
	if err != nil {
		return nil, nil, err
	}

	return eHeader, eCommit, nil
}
