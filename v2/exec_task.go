package v2

import (
	"fmt"
	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/rlp"
	w3qtypes "github.com/web3q/core/types"
	"math/big"
)

const (
	ExecTaskNoStart = 0
	ExecTaskDoing   = 1
	ExecTaskStopped = 2
)

type (
	ExecTask struct {
		targetChainId uint64
		contractAddr  common.Address
		methodName    string

		status uint64

		execFunc func(c *EthChainRelayer, value interface{}, task *ExecTask) error

		receiveCh chan interface{}
		sub       ethereum.Subscription

		cancelFunc chan struct{}
	}
)

func NewExecTask(caddr common.Address, mName string, targetChainId uint64) *ExecTask {
	return &ExecTask{
		contractAddr:  caddr,
		methodName:    mName,
		status:        ExecTaskNoStart,
		targetChainId: targetChainId,
		receiveCh:     make(chan interface{}),
		cancelFunc:    make(chan struct{}),
	}
}

func (et *ExecTask) Type() string {
	return "ExecTask"
}

func (et *ExecTask) running() {
	for {
		select {
		case <-et.receiveCh:
			//err := et.execFunc(data)
		case <-et.cancelFunc:
			return
		}
	}
}

func (manager *TaskManager) GenReceiveToken_ExecTask_OnEth(chainId uint64) *ExecTask {
	task := NewExecTask(common.Address{}, receiveFromWeb3qFunc, chainId)
	ef := func(c *EthChainRelayer, value interface{}, task *ExecTask) error {
		log := value.(*types.Log)
		web3qBlockNumber := log.BlockNumber

		// receive from web3q
		// 1. get header from we3q
		header, err := c.GetSpecificHeader(web3qBlockNumber)
		if err != nil {
			return err
		}
		fmt.Println(header)

		// 2. judge whether header already existed on light-client(eth)

		exist, err := c.IsW3qHeaderExistAtLightClient(big.NewInt(0).SetUint64(web3qBlockNumber))
		if err != nil {
			return err
		}

		if !exist {
			// 3. submit Header to Ethereum
			//eHeader, eCommit, err := PackedWeb3qHeader(header)
			//if err != nil {
			//	return err
			//}

		}

		// 4. get receipt_proof from web3q
		// 5. call receiveFromWeb3q(uint256 height, ILightClient.Proof memory proof,uint256 logIdx) on Ethereum

		return nil
	}

	task.execFunc = ef
	return task
}

func (manager *TaskManager) GenReceiveToken_ExecTask_OnWeb3q(chainId uint64) (*ExecTask, error) {
	task := NewExecTask(common.Address{}, receiveFromWeb3qFunc, chainId)
	ef := func(c *EthChainRelayer, value interface{}, task *ExecTask) error {
		log := value.(*types.Log)
		// todo: make sure enough block confirms
		tx, err := c.GenTx(task, log.TxHash, log.Index)
		if err != nil {
			return err
		}

		err = c.SubmitTx(tx)
		return err
	}
	task.execFunc = ef
	return task, nil
}

func (manager *TaskManager) GenSubmitWeb3qHeader_ExecTask_OnEth(chainId uint64) *ExecTask {
	task := NewExecTask(common.Address{}, receiveFromWeb3qFunc, chainId)
	ef := func(c *EthChainRelayer, value interface{}, task *ExecTask) error {
		// we should notify that the header is web3q's header
		header := value.(*w3qtypes.Header)
		eHeader, eCommit, err := PackedWeb3qHeader(header)
		if err != nil {
			return err
		}

		tx, err := c.GenTx(task, header.Number, eHeader, eCommit, false)
		if err != nil {
			return err
		}

		err = c.SubmitTx(tx)
		return err
	}
	task.execFunc = ef
	return nil
}

// submit epoch Header
// 0. get epoch header from we3q
// 1. encode header data
// 2. submit Header to Ethereum

func PackedWeb3qHeader(header *w3qtypes.Header) ([]byte, []byte, error) {
	cph := w3qtypes.CopyHeader(header)
	cph.Commit = nil
	eHeader, err := rlp.EncodeToBytes(cph)
	if err != nil {
		return nil, nil, err
	}

	cph = w3qtypes.CopyHeader(header)
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
