package v2

import (
	"context"
	"errors"
	"fmt"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/log"
	"math/big"
	"sync"
	"sync/atomic"
	"time"
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
	receiveBurnLog         chan interface{}
	receiveHeader          chan *types.Header
	beforeSendHeader       chan *types.Header
	SentHeader             map[uint64]bool

	status uint32
	pwg    sync.WaitGroup
	ctx    context.Context
	cf     context.CancelFunc
}

func NewScheduleTaskFromW3qToEth(name string, manager *TaskManager) (*ScheduleTask, error) {
	ctx, cancelFunc := context.WithCancel(manager.ctx)

	monitorEventTask := manager.GenMonitorEventTask(Web3qChainConf.chainId, Web3qChainConf.bridgeAddr, "SendToken")
	monitorHeaderTask := manager.GenSubHeaderMonitorTask(Web3qChainConf.chainId)

	recTokenTx := manager.GenReceiveToken_SubmitTxTask_OnEth()
	submitHeaderTask := manager.GenSubmitWeb3qHeader_SubmitTxTask_OnEth()

	// add monitor latest head
	stask := &ScheduleTask{
		taskType: ScheduleTaskType,
		name:     name,

		ethRelayer: GlobalCoordinator.GetRelayer(EthereumChainConf.chainId).(*EthChainRelayer),
		w3qRelayer: GlobalCoordinator.GetRelayer(Web3qChainConf.chainId).(*EthChainRelayer),

		sourceChain: Web3qChainConf.chainId,
		targetChain: EthereumChainConf.chainId,

		receiveBurnLog:   make(chan interface{}),
		receiveHeader:    make(chan *types.Header, 20),
		beforeSendHeader: make(chan *types.Header, 10),
		SentHeader:       make(map[uint64]bool),

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

func (s *ScheduleTask) Type() uint32 {
	return ScheduleTaskType
}

func (s *ScheduleTask) Name() string {
	return "ScheduleTask"
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

func (s *ScheduleTask) Stop() error {
	if s.Status() == ScheduleTaskRunning {
		s.cf()
		s.SetStatus(ScheduleTaskStopping)
		return nil
	}
	return fmt.Errorf("ScheduleTask::Stop() failed to send stop signal")
}

func (s *ScheduleTask) running() error {
	if s.Status() != ScheduleTaskRunning {
		return fmt.Errorf("ScheduleTask::Running() %s schedule-task already running", s.name)
	}

	for {
		select {
		case rlog := <-s.receiveBurnLog:
			logData := rlog.(*types.Log)
			// get BurnLog
			w3qHeaderNum := big.NewInt(0).SetUint64(logData.BlockNumber)
			header, err := s.w3qRelayer.GetBlockHeader(w3qHeaderNum)
			if err != nil {
				log.Error("[ScheduleTask::running()::<-s.receiveBurnLog] failed to w3qRelayer.GetBlockHeader() ", "header", w3qHeaderNum, "schedule-task", s.Name(), "err", err.Error())
				continue
			}
			log.Info("ScheduleTask::running() waiting submit header", "header", w3qHeaderNum, "schedule-task", s.Name())
			s.beforeSendHeader <- header
			go func() {
				time.Sleep(200 * time.Second)
				// todo : should waiting until submit header tx succeed
				log.Info("ScheduleTask::running() send log to submit_header_task", "header", w3qHeaderNum, "schedule-task", s.Name())
				s.sendReceiveTokenSignal <- logData
			}()

		case header := <-s.beforeSendHeader:
			log.Info("[ScheduleTask::running()::<-s.beforeSendHeader] preProcess header before sending", "header", header.Number, "schedule-task", s.Name())
			if s.SentHeader[header.Number.Uint64()] == false {

				exist, err := s.ethRelayer.IsW3qHeaderExistAtLightClient(header.Number)
				if err != nil {
					// todo how to process error
					log.Error("ScheduleTask::running() ethRelayer.IsW3qHeaderExistAtLightClient() happened error", "target-chain", s.targetChain, "schedule-task", s.Name())
					continue
				}

				if !exist {
					log.Info("ScheduleTask::running() send header to submit_header_task", "header", header.Number, "schedule-task", s.Name())
					s.sendSubmitHeaderSignal <- header
					s.SentHeader[header.Number.Uint64()] = true
				}
			}

		case <-s.ctx.Done():
			// todo : delete subscription s.MonitorBurnToken s.MonitorHeader
			// todo : delete submitTxTask
			s.SetStatus(ScheduleTaskStopped)
			return nil

		case header := <-s.receiveHeader:
			// todo: judge whether header is epoch header

			height, err := s.ethRelayer.getNextEpochHeader()
			if err != nil {
				log.Error("ScheduleTask::running() ethRelayer.getNextEpochHeader() happened error", "target-chain", s.targetChain, "schedule-task", s.Name())
				continue
			}

			if height.Cmp(header.Number) == 0 {
				s.beforeSendHeader <- header
			}
		}
	}
}

func (s *ScheduleTask) TargetChainId() uint64 {
	panic("ScheduleTask no support TargetChainId()")
	return 0
}
