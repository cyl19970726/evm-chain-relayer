package v2

import (
	"fmt"
	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/log"
	"sync"
	"sync/atomic"
)

type MonitorHeaderTask struct {
	targetChainId uint64

	sendDataCh chan *types.Header

	MonitorFunc func(c IChainRelayer) (err error)
	recCh       chan *types.Header
	sub         ethereum.Subscription
	errCh       chan error

	cancelCh chan struct{}
	status   uint32 // 0 means that the execution has not started yet, 1 means that it is executing, and 2 means that the execution is over.

	pwg sync.WaitGroup
}

func NewMonitorHeaderTask(pwg sync.WaitGroup, targetChainId uint64) *MonitorHeaderTask {
	return &MonitorHeaderTask{
		targetChainId: targetChainId,
		status:        0,
		errCh:         make(chan error),
		recCh:         make(chan *types.Header),
		cancelCh:      make(chan struct{}),
		pwg:           pwg,
	}
}

func (task *MonitorHeaderTask) SubscribeData(sendDataCh chan *types.Header) error {
	if task.sendDataCh != nil {
		return fmt.Errorf("%s MonitorHeaderTask has been subscribed", task.Name())
	}
	task.sendDataCh = sendDataCh
	return nil
}

func (task *MonitorHeaderTask) Name() string {
	return "MonitorHeaderTask"
}

func (task *MonitorHeaderTask) TargetChainId() uint64 {
	return task.targetChainId
}

func (task *MonitorHeaderTask) GetMonitorFunc() func(c IChainRelayer) (err error) {
	return task.MonitorFunc
}

func (task *MonitorHeaderTask) ExecMonitorFunc(c IChainRelayer) error {
	return task.MonitorFunc(c)
}

func (task *MonitorHeaderTask) StartMonitor() error {
	task.pwg.Add(1)
	defer task.pwg.Done()

	if atomic.LoadUint32(&task.status) != MonitorTaskNoStart {
		return fmt.Errorf("MonitorHeaderTask::StartMonitor() start monitor-task with invalid status")
	}

	atomic.StoreUint32(&task.status, MonitorTaskMonitoring)

	log.Info("MonitorHeaderTask::StartMonitor() succeed to start the monitor-contract-event task", "chainId", task.TargetChainId())
	task.Monitoring()
	return nil
}

func (task *MonitorHeaderTask) Monitoring() {

	if atomic.LoadUint32(&task.status) != MonitorTaskMonitoring {
		log.Error("monitoring with invalid status", "task-status", atomic.LoadUint32(&task.status), "chainId", task.TargetChainId())
	}

	//err := <-task.errCh
	//if err != nil {
	//	log.Error("MonitorHeaderTask::StartMonitor() monitor-contract-event task happened error", "chainId", task.TargetChainId(), , "err", err.Error())
	//	return
	//}

	log.Info("MonitorHeaderTask::Monitoring() running the monitor-contract-event task", "chainId", task.TargetChainId())

	for {
		select {
		case err := <-task.errCh:
			if err != nil {
				log.Error("MonitorHeaderTask::Monitoring() monitor-contract-event task happened error", "chainId", task.TargetChainId(), "err", err.Error())
				// Todo : to confirm whether task.stop() has high probability of producing a panic when the task.sub is nil
				task.Stop()
			}
		case data := <-task.recCh:
			log.Info("MonitorHeaderTask::Monitoring() receive event log", "chainId", task.targetChainId)
			if task.sendDataCh != nil {
				log.Info("MonitorHeaderTask::Monitoring() sending data to next processing program", "chainId", task.targetChainId)
				task.sendDataCh <- data
			}
		case err := <-task.sub.Err():
			task.errCh <- err

		case <-task.cancelCh:
			log.Info("MonitorHeaderTask::Monitoring() receive the stop-signal", "chainId", task.TargetChainId())
			task.sub.Unsubscribe()
			//close(task.errCh)
			//close(task.recCh)
			//close(task.cancelCh)
			task.SetStatus(MonitorTaskStopped)
			return
		}
	}
}

func (task *MonitorHeaderTask) Stop() error {
	log.Info("MonitorHeaderTask::Stop() send the stop-signal to the monitor-contract-event task ", "chainId", task.TargetChainId())
	if atomic.LoadUint32(&task.status) == MonitorTaskMonitoring {
		task.SetStatus(MonitorTaskStopping)
		task.cancelCh <- struct{}{}
		return nil
	}

	return fmt.Errorf("MonitorHeaderTask::Stop() failed to stop the monitor-contract-event task execution")
}

func (task *MonitorHeaderTask) SetStatus(newStatus uint32) {
	atomic.StoreUint32(&task.status, newStatus)
}

func (task *MonitorHeaderTask) Status() uint32 {
	return atomic.LoadUint32(&task.status)
}

func (manager *TaskManager) GenSubHeaderMonitorTask(targetChainId uint64) *MonitorHeaderTask {
	task := NewMonitorHeaderTask(manager.wg, targetChainId)
	ef := func(c IChainRelayer) (err error) {
		r := c.(*EthChainRelayer)
		if targetChainId != r.ChainId() {
			return fmt.Errorf("task chainId %d no match with relayer chainId %d", targetChainId, r.ChainId())
		}
		task.sub, task.recCh, err = c.(*EthChainRelayer).SubscribeLatestHeader()
		return err
	}
	task.MonitorFunc = ef

	return task
}
