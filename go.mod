module evm-chain-relayer

go 1.19

require github.com/ethereum/go-ethereum v1.10.17

require (
	github.com/StackExchange/wmi v1.2.1 // indirect
	github.com/btcsuite/btcd v0.22.0-beta // indirect
	github.com/deckarep/golang-set v1.8.0 // indirect
	github.com/go-ole/go-ole v1.2.6 // indirect
	github.com/go-stack/stack v1.8.1 // indirect
	github.com/golang/snappy v0.0.4 // indirect
	github.com/google/uuid v1.3.0 // indirect
	github.com/gorilla/websocket v1.5.0 // indirect
	github.com/rjeczalik/notify v0.9.1 // indirect
	github.com/shirou/gopsutil v3.21.4-0.20210419000835-c7a38de76ee5+incompatible // indirect
	github.com/syndtr/goleveldb v1.0.1-0.20210819022825-2ae1ddf74ef7 // indirect
	github.com/tklauser/go-sysconf v0.3.10 // indirect
	github.com/tklauser/numcpus v0.5.0 // indirect
	golang.org/x/crypto v0.0.0-20220513210258-46612604a0f9 // indirect
	golang.org/x/sys v0.0.0-20220513210249-45d2b4557a2a // indirect
	gopkg.in/natefinch/npipe.v2 v2.0.0-20160621034901-c1b8fa8bdcce // indirect
)

//replace github.com/web3q v1.10.17 => ../Work/EthStorage/ExternalCall/go-ethereum
//replace github.com/web3q v1.10.17 => github.com/QuarkChain/go-ethereum tm_w3q_bridge_adoption
replace github.com/ethereum/go-ethereum v1.10.17 => github.com/QuarkChain/go-ethereum v1.9.23-0.20230321092345-0673454957e2
