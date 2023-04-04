package v2

import (
	"fmt"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/rlp"
	"math/big"
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
		sourceChainId uint64
		targetChainId uint64
		contractAddr  common.Address
		contractName  string
		methodName    string

		status uint32

		submitTxFunc func(source *EthChainRelayer, target *EthChainRelayer, value interface{}, task *SubmitTxTask) (*types.Transaction, error)

		pwg       sync.WaitGroup
		receiveCh chan interface{}
		cancelCh  chan struct{}
	}
)

func NewSubmitTxTask(caddr common.Address, cName, mName string, sourceChainId uint64, targetChainId uint64, pwg sync.WaitGroup) *SubmitTxTask {
	return &SubmitTxTask{
		contractAddr:  caddr,
		contractName:  cName,
		methodName:    mName,
		status:        SubmitTxTaskNoStart,
		sourceChainId: sourceChainId,
		targetChainId: targetChainId,
		receiveCh:     make(chan interface{}),
		cancelCh:      make(chan struct{}),
		pwg:           pwg,
	}
}

func (st *SubmitTxTask) Type() uint32 {
	return SubmitTxTaskType
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
			sr := GlobalCoordinator.GetRelayer(et.sourceChainId).(*EthChainRelayer)
			tr := GlobalCoordinator.GetRelayer(et.targetChainId).(*EthChainRelayer)
			if sr == nil {
				return fmt.Errorf("chainRelayer %d no exist", et.sourceChainId)
			}
			if tr == nil {
				return fmt.Errorf("chainRelayer %d no exist", et.targetChainId)
			}

			tx, err := et.submitTxFunc(sr, tr, data, et)
			// todo : how to deal failed tx
			if err != nil {
				log.Error("SubmitTxTask::running() failed to submitTx", "chainId", et.TargetChainId(), "methodName", et.methodName, "error", err.Error())
				fmt.Printf("tx: %v", *tx)
				continue
			}

			log.Info("SubmitTxTask::running() succeed to submitTx", "chainId", et.TargetChainId(), "txhash", tx.Hash(), "methodName", et.methodName)

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

type Proof struct {
	Value     []byte
	ProofPath []byte
	HpKey     []byte
}

func (manager *TaskManager) GenReceiveToken_SubmitTxTask_OnEth() *SubmitTxTask {
	task := NewSubmitTxTask(EthereumChainConf.bridgeAddr, EthereumBridgeContract, receiveFromWeb3qFunc, Web3qChainConf.chainId, EthereumChainConf.chainId, manager.wg)
	ef := func(source *EthChainRelayer, target *EthChainRelayer, value interface{}, task *SubmitTxTask) (*types.Transaction, error) {
		log := value.(*types.Log)
		// 4. get receipt_proof from web3q
		proof, err := source.GetReceiptProof(log.TxHash)
		if err != nil {
			return nil, err
		}

		p := &Proof{
			Value:     proof.ReceiptValue,
			ProofPath: proof.ReceiptValue,
			HpKey:     proof.ReceiptKey,
		}

		logIndex := big.NewInt(0).SetUint64(uint64(log.Index))
		fmt.Printf("===%d ==== %d====", log.Index, logIndex.Uint64())
		tx, err := target.GenTx(task, big.NewInt(0).SetUint64(log.BlockNumber), p, logIndex)
		if err != nil {
			return tx, err
		}
		err = target.SubmitTx(tx)
		if err != nil {
			return tx, err
		}

		return tx, nil
	}

	task.submitTxFunc = ef
	return task
}

func (manager *TaskManager) GenSubmitWeb3qHeader_SubmitTxTask_OnEth() *SubmitTxTask {
	task := NewSubmitTxTask(EthereumChainConf.lightClientAddr, LightClientContract, SubmitHeaderFunc, Web3qChainConf.chainId, EthereumChainConf.chainId, manager.wg)
	ef := func(source *EthChainRelayer, target *EthChainRelayer, value interface{}, task *SubmitTxTask) (*types.Transaction, error) {
		web3qHeader := value.(*types.Header)

		//3. submit Header to Ethereum
		eHeader, eCommit, err := PackedWeb3qHeader(web3qHeader)
		if err != nil {
			return nil, err
		}

		// todo: getNonce

		tx, err := target.GenTx(task, web3qHeader.Number, eHeader, eCommit, false)
		if err != nil {
			return tx, err
		}

		err = target.SubmitTx(tx)
		if err != nil {
			return tx, err
		}

		// todo : set the current txHash for exec_task , and track the tx

		return tx, nil
	}

	task.submitTxFunc = ef
	return task
}

const CONFIRMS = 2
const BlockInternalSecond = 10
const RetryTimes = 3

func (manager *TaskManager) GenReceiveToken_SubmitTxTask_OnWeb3q() *SubmitTxTask {
	task := NewSubmitTxTask(Web3qChainConf.bridgeAddr, Web3qBridgeContract, receiveFromEthFunc, EthereumChainConf.chainId, Web3qChainConf.chainId, manager.wg)
	ef := func(source *EthChainRelayer, target *EthChainRelayer, value interface{}, task *SubmitTxTask) (*types.Transaction, error) {
		logData := value.(*types.Log)

		sourceExpectBN := logData.BlockNumber + CONFIRMS
		retryNonce := 0
		for {
			sourceLatestBN := source.latestHeaderNum.Uint64()
			log.Info("submit-task submitting tx:: waiting enough confirms", "current-block", sourceLatestBN, "expect-block", sourceExpectBN)
			if sourceLatestBN >= sourceExpectBN {
				tx, err := target.GenTx(task, logData.TxHash, big.NewInt(0))
				if err != nil {
					log.Error("submit-task submitting tx:: generate tx err ", "submit-task", task.Name(), "targetChain", task.TargetChainId())
					return tx, err
				}
				err = target.SubmitTx(tx)
				if err != nil && retryNonce <= RetryTimes {
					log.Error("submit-task submitting tx:: happen error and retrying", "submit-task", task.Name(), "targetChain", task.TargetChainId())
					retryNonce++
					continue
				}

				return tx, err
			}

			//var ss int = int(sourceExpectBN-sourceLatestBN) * BlockInternalSecond
			time.Sleep(BlockInternalSecond * time.Second)
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
