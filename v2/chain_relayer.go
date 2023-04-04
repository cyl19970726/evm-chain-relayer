package v2

import (
	"context"
	"errors"
	"fmt"
	"github.com/ethereum/go-ethereum/ethclient"
)

const (
	ChainRelayerNoStart = 0
	ChainRelayerDoing   = 1
	ChainRelayerStopped = 2
)

type IChainRelayer interface {
	ChainId() uint64
	SendMonitorTask(task IMonitorTask) error
	Start() error
	Stop() error
}

type IChainClient interface {
	ChainId() uint64
	EthWsClient() *ethclient.Client
	EthHttpClient() *ethclient.Client
	Web3qWsClient() *ethclient.Client
	Web3qHttpClient() *ethclient.Client
}
type EthChainClient struct {
	chainId    uint64
	wsClient   *ethclient.Client
	httpClient *ethclient.Client
}

func (e *EthChainClient) ChainId() uint64 {
	return e.chainId
}

func (e *EthChainClient) EthWsClient() *ethclient.Client {
	return e.wsClient
}

func (e *EthChainClient) EthHttpClient() *ethclient.Client {
	return e.httpClient
}

func (e *EthChainClient) Web3qWsClient() *ethclient.Client {
	panic("EthChainClient no implement w3qClient interface")
}

func (e *EthChainClient) Web3qHttpClient() *ethclient.Client {
	panic("EthChainClient no implement w3qClient interface")
}

func (c *EthChainClient) Close() {
	c.wsClient.Close()
	c.httpClient.Close()
}

type Web3qChainClient struct {
	chainId    uint64
	wsClient   *ethclient.Client
	httpClient *ethclient.Client
}

func (e *Web3qChainClient) ChainId() uint64 {
	return e.chainId
}

func (w *Web3qChainClient) EthWsClient() *ethclient.Client {
	panic("Web3qChainClient no implement EthChainClient interface")
}

func (w *Web3qChainClient) EthHttpClient() *ethclient.Client {
	panic("Web3qChainClient no implement EthChainClient interface")
}

func (w *Web3qChainClient) Web3qWsClient() *ethclient.Client {
	return w.wsClient
}

func (w *Web3qChainClient) Web3qHttpClient() *ethclient.Client {
	return w.httpClient
}

func (c *Web3qChainClient) Close() {
	c.wsClient.Close()
	c.httpClient.Close()
}

func NewEthChainClient(httpUrl, wsUrl string, ctx context.Context) (*EthChainClient, error) {

	if httpUrl == "" || wsUrl == "" {
		return nil, errors.New("httpUrl and wsUrl are empty")
	}

	var httpClient, wsClient *ethclient.Client
	var err error
	if httpUrl != "" {
		httpClient, err = ethclient.DialContext(ctx, httpUrl)
		if err != nil {
			return nil, err
		}
	}

	if wsUrl != "" {
		wsClient, err = ethclient.DialContext(ctx, wsUrl)
		if err != nil {
			panic(err)
			return nil, err
		}
	}
	hchainId, err := httpClient.ChainID(ctx)
	if err != nil {
		return nil, err
	}
	wchainId, err := wsClient.ChainID(ctx)
	if err != nil {
		return nil, err
	}

	if wchainId.Cmp(hchainId) != 0 {
		return nil, fmt.Errorf("chainId-%d of ws-client is different with chainId-%d of http-client", wchainId.Uint64(), hchainId.Uint64())
	}

	return &EthChainClient{chainId: wchainId.Uint64(), wsClient: wsClient, httpClient: httpClient}, nil
}

func NewW3qChainClient(httpUrl, wsUrl string, ctx context.Context) (*Web3qChainClient, error) {

	if httpUrl == "" && wsUrl == "" {
		return nil, errors.New("httpUrl and wsUrl are empty")
	}

	var httpClient, wsClient *ethclient.Client
	var err error
	if httpUrl != "" {
		httpClient, err = ethclient.DialContext(ctx, httpUrl)
		if err != nil {
			return nil, err
		}
	}

	if wsUrl != "" {
		wsClient, err = ethclient.DialContext(ctx, wsUrl)
		if err != nil {
			return nil, err
		}
	}
	return &Web3qChainClient{wsClient: wsClient, httpClient: httpClient}, nil
}
