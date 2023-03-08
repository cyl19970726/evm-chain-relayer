package relayer

import (
	"context"
	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"time"
)

// 如果receive chan 不为nil ，则代表这个event启用单独的处理线程，否则一律使用ChainOperator.LogsReceiveChan
type HandleEventTask struct {
	start                  bool
	address                common.Address
	eventName              string
	independentReceiveChan chan types.Log
	sub                    ethereum.Subscription
	handleFunc             func(log2 types.Log)

	ctx        context.Context
	cancleFunc func()
}

func NewHandleEventTask(address common.Address, eventName string, recChan chan types.Log, subscription ethereum.Subscription, sCtx context.Context, cf func()) *HandleEventTask {
	return &HandleEventTask{start: true, address: address, eventName: eventName, independentReceiveChan: recChan, sub: subscription, ctx: sCtx, cancleFunc: cf}
}

func (he *HandleEventTask) setIndependentReceiveChan(c chan types.Log) {
	he.independentReceiveChan = c
}

func (task *HandleEventTask) running(co *ChainOperator) {
	for {
		select {
		case log := <-task.independentReceiveChan:
			co.config.logger.Info("receive log", "event", task.eventName, "Log Info", log)
			if task.handleFunc != nil {
				task.handleFunc(log)
			}
		case err := <-task.sub.Err():
			co.config.logger.Error("receive error", "event", task.eventName, "err", err)
			co.reSubscribeEvent(task)
		case <-task.ctx.Done():
			delete(co.contracts[task.address].HandleEventList, co.contracts[task.address].getEventId(task.eventName))
			task.sub.Unsubscribe()
			task.start = false

			co.config.logger.Info("listening task quit ", "event", task.eventName)
			return
		default:
			co.config.logger.Info("listen task working", "event", task.eventName)
			time.Sleep(10 * time.Second)

		}
	}
}
