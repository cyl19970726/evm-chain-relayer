package v2

type Task interface {
	Type() uint32
	Name() string
	TargetChainId() uint64
	Status() uint32
}
type TaskPool struct {
	monitorQueue  []IMonitorTask
	txQueue       []*SubmitTxTask
	scheduleQueue []*ScheduleTask
}

func NewTaskPool() *TaskPool {
	mqueue := make([]IMonitorTask, 0)
	equeue := make([]*SubmitTxTask, 0)
	squeue := make([]*ScheduleTask, 0)
	return &TaskPool{monitorQueue: mqueue, txQueue: equeue, scheduleQueue: squeue}
}

func (pool *TaskPool) AddMonitorTask(t IMonitorTask) *TaskPool {
	pool.monitorQueue = append(pool.monitorQueue, t)
	return pool
}

func (pool *TaskPool) AddSubmitTxTask(t *SubmitTxTask) *TaskPool {
	pool.txQueue = append(pool.txQueue, t)
	return pool
}

func (pool *TaskPool) AddScheduleTask(t *ScheduleTask) *TaskPool {
	pool.scheduleQueue = append(pool.scheduleQueue, t)
	return pool
}

// how to process the outcome of transaction execution
// case1 : succeed
// case2 : failed
