package v2

type Task interface {
	TargetChainId() uint64
	Name() uint // execTask or listenTask
}
type TaskPool struct {
	mqueue         []*MonitorTask
	equeue         []*Task
	executableTask []*Task // queue for ready to execute
	pendingTask    []*Task // queue for executing task
}

func NewTaskPool() *TaskPool {
	mqueue := make([]*MonitorTask, 0)
	equeue := make([]*Task, 0)
	executableTask := make([]*Task, 0)
	pendingTask := make([]*Task, 0)
	return &TaskPool{mqueue: mqueue, equeue: equeue, executableTask: executableTask, pendingTask: pendingTask}
}

func (pool *TaskPool) AddMonitorTask(t *MonitorTask) {
	pool.mqueue = append(pool.mqueue, t)
}

func (pool *TaskPool) AddExecTask(t *Task) {
	pool.executableTask = append(pool.executableTask, t)
}

func (pool *TaskPool) popExecutableTask() *Task {
	task := pool.executableTask[0]
	pool.executableTask = pool.executableTask[1:]
	pool.pendingTask = append(pool.pendingTask, task)
	return task
}

// how to process the outcome of transaction execution
// case1 : succeed
// case2 : failed
