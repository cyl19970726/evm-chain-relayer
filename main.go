package main

import (
	"evm-chain-relayer/v2"
	"time"
)

func main() {
	// 1. Monitor Subscription
	go v2.GlobalCoordinator.Start()

	time.Sleep(1000 * time.Second)

	v2.GlobalCoordinator.Stop()
	//fmt.Println("Listening")
	//v2.GlobalCoordinator.Stop()
}
