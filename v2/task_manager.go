package v2

import (
	"context"
	"errors"
	"github.com/ethereum/go-ethereum/log"
	"sync"
	"sync/atomic"
)

const (
	TaskManagerNoStart  = 0
	TaskManagerDoing    = 1
	TaskManagerStopping = 2
	TaskManagerStopped  = 3
)

const (
	MonitorTaskType = 0
	SubmitTxTaskType
	ScheduleTaskType
)

// send task to
type TaskManager struct {
	*TaskPool

	wg         sync.WaitGroup
	ctx        context.Context
	cancelFunc context.CancelFunc

	status uint32
}

func NewTaskManager(ctx context.Context) *TaskManager {
	taskPool := NewTaskPool()
	wg := sync.WaitGroup{}
	sctx, cancelFunc := context.WithCancel(ctx)
	return &TaskManager{TaskPool: taskPool, wg: wg, ctx: sctx, cancelFunc: cancelFunc, status: TaskManagerNoStart}
}

func (manager *TaskManager) Start() error {

	if atomic.LoadUint32(&manager.status) != TaskManagerNoStart {
		return errors.New("TaskManager Start with invalid status")
	}
	atomic.StoreUint32(&manager.status, TaskManagerDoing)

	log.Info("TaskManager::Start() taskManager start succeed")
	return manager.running()
}

func (manager *TaskManager) running() error {
	if atomic.LoadUint32(&manager.status) != TaskManagerDoing {
		return errors.New("TaskManager running with invalid status")
	}

	for _, stask := range manager.scheduleQueue {
		go func() {
			err := stask.Start()
			if err != nil {
				panic(err)
			}
		}()
	}

	for _, mtask := range manager.monitorQueue {
		err := GlobalCoordinator.SendTaskToRelayer(mtask)
		if err != nil {
			panic(err)
			return err
		}
	}

	for _, ttask := range manager.txQueue {
		go func() {
			err := ttask.Start()
			if err != nil {
				panic(err)
			}
		}()
	}

	for {
		select {
		case <-manager.ctx.Done():
			log.Info("TaskManager::running() TaskManager received stop-signal and will be done")

			for _, ttask := range manager.txQueue {
				ttask.Stop()
			}

			for _, stask := range manager.scheduleQueue {
				stask.Stop()
			}

			for _, mtask := range manager.monitorQueue {
				if mtask.Status() == MonitorTaskMonitoring {
					mtask.Stop()
				}
			}

			manager.wg.Wait()
			manager.SetStatus(TaskManagerStopped)
			return nil
		}
	}

}

func (manager *TaskManager) Stop() error {
	if manager.Status() == TaskManagerDoing {
		manager.cancelFunc()
		manager.SetStatus(TaskManagerStopping)
	}

	return errors.New("TaskManager stop with invalid status")
	return nil
}

func (manager *TaskManager) StopTaskBySpecificChainId(chainId uint64) {
	for _, task := range manager.monitorQueue {
		if task.TargetChainId() == chainId && task.Status() == MonitorTaskMonitoring {
			task.Stop()
			// todo : move the stopped task to stopped-queue or vanish it
		}
	}
}

func (manager *TaskManager) Status() uint32 {
	return atomic.LoadUint32(&manager.status)
}

func (manager *TaskManager) SetStatus(newStatus uint32) {
	atomic.StoreUint32(&manager.status, newStatus)
}
