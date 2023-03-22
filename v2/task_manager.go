package v2

import (
	"context"
	"errors"
	"github.com/ethereum/go-ethereum/log"
	"sync"
	"sync/atomic"
)

const (
	TaskManagerNoStart = 0
	TaskManagerDoing   = 1
	TaskManagerStopped = 2
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

	for _, mtask := range manager.mqueue {
		err := GlobalCoordinator.SendTaskToRelayer(mtask)
		if err != nil {
			panic(err)
			return err
		}
	}

	//for _, mtask := range manager.mqueue {
	//	manager.wg.Add(1)
	//	go func(taskManager *TaskManager, mt *MonitorTask) {
	//		defer taskManager.wg.Done()
	//		mt.StartMonitor()
	//	}(manager, mtask)
	//}

	for {
		select {
		case <-manager.ctx.Done():
			log.Info("TaskManager::running() TaskManager received stop-signal and will be done")
			for _, mtask := range manager.mqueue {
				if atomic.LoadUint32(&mtask.status) == MonitorTaskMonitoring {
					mtask.Stop()
				}
			}

			manager.wg.Wait()
			atomic.StoreUint32(&manager.status, TaskManagerStopped)
			return nil
		}
	}

}

func (manager *TaskManager) Stop() error {
	if atomic.LoadUint32(&manager.status) != TaskManagerDoing {
		return errors.New("TaskManager stop with invalid status")
	}
	manager.cancelFunc()
	return nil
}

func (manager *TaskManager) StopTaskBySpecificChainId(chainId uint64) {
	for _, task := range manager.mqueue {
		if task.TargetChainId() == chainId && atomic.LoadUint32(&task.status) == MonitorTaskMonitoring {
			task.Stop()
			// todo : move the stopped task to stopped-queue or vanish it
		}
	}
}
