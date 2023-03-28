package v2

import (
	"context"
	"errors"
	"fmt"
	"github.com/ethereum/go-ethereum/core/types"
	"math/big"
	"sync"
	"sync/atomic"
)

const (
	ScheduleTaskNoStart  = 0
	ScheduleTaskRunning  = 1
	ScheduleTaskStopping = 2
	ScheduleTaskStopped  = 3
)

type ScheduleTask struct {
	taskType uint64
	name     string

	ethRelayer *EthChainRelayer
	w3qRelayer *EthChainRelayer

	sourceChain uint64
	targetChain uint64

	MonitorHeader    *MonitorHeaderTask
	MonitorBurnToken *MonitorTask

	SubmitHeaderTx       *SubmitTxTask
	SubmitReceiveTokenTX *SubmitTxTask

	sendSubmitHeaderSignal chan interface{}
	sendReceiveTokenSignal chan interface{}
	receiveBurnLog         chan *types.Log
	receiveHeader          chan *types.Header

	status uint32
	pwg    sync.WaitGroup
	ctx    context.Context
	cf     context.CancelFunc
}

func NewScheduleTaskFromW3qToEth(name string, manager *TaskManager) (*ScheduleTask, error) {
	ctx, cancelFunc := context.WithCancel(manager.ctx)

	monitorEventTask := manager.GenMonitorEventTask(Web3qChainConf.chainId, Web3qChainConf.bridgeAddr, "sendToken")
	monitorHeaderTask := manager.GenSubHeaderMonitorTask(Web3qChainConf.chainId)

	recTokenTx := manager.GenReceiveToken_SubmitTxTask_OnEth()
	submitHeaderTask := manager.GenSubmitWeb3qHeader_SubmitTxTask_OnEth()

	// add monitor latest head
	stask := &ScheduleTask{
		taskType: ScheduleTaskType,
		name:     name,

		sourceChain: Web3qChainConf.chainId,
		targetChain: EthereumChainConf.chainId,

		receiveBurnLog: make(chan *types.Log),
		receiveHeader:  make(chan *types.Header),

		status: ScheduleTaskNoStart,
		ctx:    ctx,
		cf:     cancelFunc,
	}
	err := monitorEventTask.SubscribeData(stask.receiveBurnLog)
	if err != nil {
		return nil, err
	}

	err = monitorHeaderTask.SubscribeData(stask.receiveHeader)
	if err != nil {
		return nil, err
	}

	err = stask.SubscribeHeader(submitHeaderTask.receiveCh)
	if err != nil {
		return nil, err
	}

	err = stask.SubscribeBurnLog(recTokenTx.receiveCh)
	if err != nil {
		return nil, err
	}

	manager.
		AddMonitorTask(monitorEventTask).AddMonitorTask(monitorHeaderTask).
		AddScheduleTask(stask).
		AddSubmitTxTask(submitHeaderTask).AddSubmitTxTask(recTokenTx)

	return stask, nil
}

func (s *ScheduleTask) SubscribeHeader(sendSubmitHeaderSignal chan interface{}) error {
	if s.sendSubmitHeaderSignal != nil {
		return errors.New("ScheduleTask:: failed to SubscribeHeader due to s.sendSubmitHeaderSignal != nil")
	}
	s.sendSubmitHeaderSignal = sendSubmitHeaderSignal
	return nil
}

func (s *ScheduleTask) SubscribeBurnLog(sendReceiveTokenSignal chan interface{}) error {
	if s.sendReceiveTokenSignal != nil {
		return errors.New("ScheduleTask:: failed to SubscribeBurnLog due to s.sendReceiveTokenSignal != nil")
	}

	s.sendReceiveTokenSignal = sendReceiveTokenSignal
	return nil
}

func (s *ScheduleTask) Status() uint32 {
	return atomic.LoadUint32(&s.status)
}

func (s *ScheduleTask) SetStatus(newStatus uint32) {
	atomic.StoreUint32(&s.status, newStatus)
}

func (s *ScheduleTask) Start() error {
	s.pwg.Add(1)
	defer s.pwg.Done()
	if s.Status() != ScheduleTaskNoStart {
		return fmt.Errorf("ScheduleTask::Start() %s schedule-task already started", s.name)
	}

	s.SetStatus(ScheduleTaskRunning)
	return s.running()
}

func (s *ScheduleTask) running() error {
	if s.Status() != ScheduleTaskRunning {
		return fmt.Errorf("ScheduleTask::Running() %s schedule-task already running", s.name)
	}

	for {
		select {
		case log := <-s.receiveBurnLog:
			// get BurnLog
			w3qHeaderNum := big.NewInt(0).SetUint64(log.BlockNumber)
			exist, err := s.ethRelayer.IsW3qHeaderExistAtLightClient(w3qHeaderNum)
			if err != nil {
				// todo how to process error
				panic(err)
			}
			if !exist {
				header, err := s.w3qRelayer.GetBlockHeader(w3qHeaderNum)
				if err != nil {
					panic(err)
				}
				s.receiveHeader <- header
			}

			// todo : should waiting until submit header tx succeed
			s.sendReceiveTokenSignal <- log

		case header := <-s.receiveHeader:
			// todo: judge whether header is epoch header
			s.sendSubmitHeaderSignal <- header
		case <-s.ctx.Done():
			// todo : delete subscription s.MonitorBurnToken s.MonitorHeader
			// todo : delete submitTxTask
			s.SetStatus(ScheduleTaskStopped)
			return nil
		}
	}
}

func (s *ScheduleTask) Stop() error {
	if s.Status() == ScheduleTaskRunning {
		s.cf()
		s.SetStatus(ScheduleTaskStopping)
		return nil
	}
	return fmt.Errorf("ScheduleTask::Stop() failed to send stop signal")
}
