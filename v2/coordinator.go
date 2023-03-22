package v2

import (
	"context"
	"fmt"
	"github.com/ethereum/go-ethereum/log"
	"os"
	"sync"
	"sync/atomic"
)

const (
	CoordinatorNoStart = 0
	CoordinatorDoing   = 1
	CoordinatorStopped = 2
)

var GlobalCoordinator *Coordinator

func init() {
	initContractConf()
	GlobalCoordinator = NewCoordinator(4)
	//web3qRelayer, err := NewChainRelayer("/Users/chenyanlong/Work/go-ethereum/cmd/geth/data_val_0/keystore/UTC--2022-06-07T04-15-42.696295000Z--96f22a48dcd4dfb99a11560b24bee02f374ca77d", "123", Web3qChainConf)
	//if err != nil {
	//	panic(err)
	//}

	ethRelayer, err := NewEthChainRelayer(GlobalCoordinator.ctx, "/Users/chenyanlong/Work/go-ethereum/cmd/geth/data_val_0/keystore/UTC--2022-06-07T04-15-42.696295000Z--96f22a48dcd4dfb99a11560b24bee02f374ca77d", "123", EthereumChainConf)
	if err != nil {
		panic(err)
	}

	//GlobalCoordinator.AddChainRelayer(web3qRelayer)
	GlobalCoordinator.AddChainRelayer(ethRelayer)

	eventMonitorTask := GlobalCoordinator.taskManager.GenMonitorEventTask(EthereumChainConf.chainId, EthereumChainConf.bridgeAddr, "sendToken")
	GlobalCoordinator.AddTaskIntoTaskPool(eventMonitorTask)

}

type Coordinator struct {
	relayers    map[uint64]IChainRelayer
	taskManager *TaskManager

	status     uint32
	ctx        context.Context
	cancelFunc context.CancelFunc

	wg    sync.WaitGroup
	errCh chan error

	log.Logger
}

func NewCoordinator(logLevel int) *Coordinator {

	glogger := log.NewGlogHandler(log.StreamHandler(os.Stderr, log.TerminalFormat(false)))
	glogger.Verbosity(log.Lvl(logLevel))
	log.Root().SetHandler(glogger)

	relayers := make(map[uint64]IChainRelayer)
	ctx, cf := context.WithCancel(context.Background())

	// new taskManager
	taskManager := NewTaskManager(ctx)
	return &Coordinator{Logger: log.Root(), ctx: ctx, cancelFunc: cf, relayers: relayers, taskManager: taskManager, status: CoordinatorNoStart, wg: sync.WaitGroup{}, errCh: make(chan error)}
}

func (c *Coordinator) Start() error {
	if atomic.LoadUint32(&c.status) != CoordinatorNoStart {
		return fmt.Errorf("Coordinator::Start() with invalid status [%d]", atomic.LoadUint32(&c.status))
	}
	atomic.StoreUint32(&c.status, CoordinatorDoing)
	return c.Running()
}

func (c *Coordinator) Running() error {
	if atomic.LoadUint32(&c.status) != CoordinatorDoing {
		return fmt.Errorf("Coordinator::Running() with invalid status [%d]", atomic.LoadUint32(&c.status))
	}

	for _, relayer := range c.relayers {
		c.wg.Add(1)
		go func(c *Coordinator) {
			defer c.wg.Done()
			err := relayer.Start()
			if err != nil {
				log.Error("Coordinator::Start() failed to start relayer", "chainId", relayer.ChainId(), "err", err.Error())
				c.errCh <- err
				return
			}
		}(c)
	}

	c.wg.Add(1)
	go func(c *Coordinator) {
		defer c.wg.Done()
		err := c.taskManager.Start()
		if err != nil {
			log.Error("Coordinator::Start() failed to start taskManager", "err", err.Error())
			c.errCh <- err
			return
		}
	}(c)

	for {
		select {
		case err := <-c.errCh:
			if err != nil {
				c.Stop()
			}
		case <-c.ctx.Done():
			log.Info("Coordinator::Running() coordinator receive stop-signal and will be done")
			atomic.StoreUint32(&c.status, CoordinatorStopped)
			return nil
		}
	}
	return nil
}

func (c *Coordinator) Stop() {
	log.Info("Coordinator::Stop() coordinator send stop-signal")
	c.cancelFunc()
	c.wg.Wait()
}

// SendTaskToRelayer will be invoked by taskManager
func (c *Coordinator) SendTaskToRelayer(task *MonitorTask) error {
	relayer := c.relayers[task.TargetChainId()]
	if relayer == nil {
		return fmt.Errorf("the chain-relayer corresponding to the task-chainId [%d] no exists", task.TargetChainId())
	}
	return relayer.SendMonitorTask(task)
}

func (c *Coordinator) AddChainRelayer(relayer IChainRelayer) {
	c.relayers[relayer.ChainId()] = relayer
}

// todo
func (c *Coordinator) removeChainRelayer() {

}

func (c *Coordinator) AddTaskIntoTaskPool(task *MonitorTask) {
	c.taskManager.AddMonitorTask(task)
}
