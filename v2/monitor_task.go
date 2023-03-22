package v2

import (
	"fmt"
	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/log"
	"sync"
	"sync/atomic"
)

const (
	MonitorTaskNoStart    = 0
	MonitorTaskMonitoring = 1
	MonitorTaskStopped    = 2
)

type MonitorTask struct {
	targetChainId uint64
	contractAddr  common.Address
	eventName     string
	eventId       common.Hash

	MonitorFunc func(c IChainRelayer) (err error)
	recCh       chan types.Log
	sub         ethereum.Subscription
	errCh       chan error

	cancelCh chan struct{}
	status   uint32 // 0 means that the execution has not started yet, 1 means that it is executing, and 2 means that the execution is over.

	pwg sync.WaitGroup
}

func NewMonitorTask(pwg sync.WaitGroup, targetChainId uint64, contractAddr common.Address, eventName string, eventId common.Hash) *MonitorTask {
	return &MonitorTask{

		targetChainId: targetChainId,
		contractAddr:  contractAddr,
		eventName:     eventName,
		eventId:       eventId,
		status:        0,
		errCh:         make(chan error),
		recCh:         make(chan types.Log),
		cancelCh:      make(chan struct{}),
		pwg:           pwg,
	}
}

func (task *MonitorTask) Name() string {
	return "MonitorTask"
}

func (task *MonitorTask) TargetChainId() uint64 {
	return task.targetChainId
}

func (task *MonitorTask) StartMonitor() error {
	task.pwg.Add(1)
	defer task.pwg.Done()

	if atomic.LoadUint32(&task.status) != MonitorTaskNoStart {
		return fmt.Errorf("MonitorTask::StartMonitor() start monitor-task with invalid status")
	}

	atomic.StoreUint32(&task.status, MonitorTaskMonitoring)

	log.Info("MonitorTask::StartMonitor() succeed to start the monitor-contract-event task", "chainId", task.TargetChainId(), "contract", task.contractAddr, "event", task.eventName)
	task.Monitoring()
	return nil
}

func (task *MonitorTask) Monitoring() {

	if atomic.LoadUint32(&task.status) != MonitorTaskMonitoring {
		log.Error("monitoring with invalid status", "task-status", atomic.LoadUint32(&task.status), "chainId", task.TargetChainId(), "contract", task.contractAddr, "event", task.eventName)
	}

	//err := <-task.errCh
	//if err != nil {
	//	log.Error("MonitorTask::StartMonitor() monitor-contract-event task happened error", "chainId", task.TargetChainId(), "contract", task.contractAddr, "event", task.eventName, "err", err.Error())
	//	return
	//}

	log.Info("MonitorTask::Monitoring() running the monitor-contract-event task", "chainId", task.TargetChainId(), "contract", task.contractAddr, "event", task.eventName)

	for {
		select {
		case err := <-task.errCh:
			if err != nil {
				log.Error("MonitorTask::Monitoring() monitor-contract-event task happened error", "chainId", task.TargetChainId(), "contract", task.contractAddr, "event", task.eventName, "err", err.Error())
				// Todo : to confirm whether task.stop() has high probability of producing a panic when the task.sub is nil
				task.Stop()
			}
		case data := <-task.recCh:
			log.Info("MonitorTask::Monitoring() receive event log", "chainId", task.targetChainId, "event", task.eventName, "Address", data.Topics[0].Hex(), "topics", data.Topics[1:])
			// todo: next process of log data
			// todo: execTask can add a subscription into MonitorTask
		case err := <-task.sub.Err():
			task.errCh <- err

		case <-task.cancelCh:
			log.Info("MonitorTask::Monitoring() receive the stop-signal", "chainId", task.TargetChainId(), "contract", task.contractAddr, "event", task.eventName)
			task.sub.Unsubscribe()
			//close(task.errCh)
			//close(task.recCh)
			//close(task.cancelCh)
			return
		}
	}
}

func (task *MonitorTask) Stop() {
	log.Info("MonitorTask::Stop() send the stop-signal to the monitor-contract-event task ", "chainId", task.TargetChainId(), "contract", task.contractAddr, "event", task.eventName)
	if atomic.LoadUint32(&task.status) == MonitorTaskMonitoring {
		task.cancelCh <- struct{}{}
		atomic.StoreUint32(&task.status, MonitorTaskStopped)
	} else {
		log.Error("failed to stop the monitor-contract-event task execution", "task-status", atomic.LoadUint32(&task.status), "chainId", task.TargetChainId(), "contract", task.contractAddr, "event", task.eventName)
	}

}

func (manager *TaskManager) GenMonitorEventTask(targetChainId uint64, address common.Address, eventName string) *MonitorTask {
	eventId := GlobalContractInfo.GetContractEventId(targetChainId, address, eventName)
	// todo: how to judge the eventId validity
	task := NewMonitorTask(manager.wg, targetChainId, address, eventName, eventId)
	ef := func(c IChainRelayer) (err error) {
		task.sub, err = c.(*EthChainRelayer).SubscribeEvent(address, eventId, task.recCh)
		return err
	}
	task.MonitorFunc = ef
	return task
}
