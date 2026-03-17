package leaderelection

import (
	"context"

	election "git.server.lan/pkg/leader-election-go"
	clientv3 "go.etcd.io/etcd/client/v3"
)

const appName = "metric-sherlock"

// EtcdElector представляет собой реализацию интерфейса LeaderElector с использованием подключения к etcd
type EtcdElector struct {
	manager election.RunnableManager
	onStart []OnLeaderStartCallback
	onStop  []OnLeaderStopCallback
}

// NewEtcdElector конструктор для EtcdElector
func NewEtcdElector(ctx context.Context, etcdClient *clientv3.Client) (*EtcdElector, error) {
	elector := &EtcdElector{}

	manager, err := election.NewManager(ctx, etcdClient, appName, election.Callbacks{
		OnLeaderStart: elector.onLeaderStart,
		OnLeaderStop:  elector.onLeaderStop,
	})

	if err != nil {
		return nil, err
	}

	elector.manager = manager
	return elector, nil
}

func (e *EtcdElector) onLeaderStart() {
	for _, onStart := range e.onStart {
		go onStart()
	}
}

func (e *EtcdElector) onLeaderStop() {
	for _, onStop := range e.onStop {
		go onStop()
	}
}

// Run запускает цикл election для получения лидерства через etcd
func (e *EtcdElector) Run() error {
	return e.manager.Run()
}

// AddOnStart регистрирует callback, который вызывается при получении лидерства
func (e *EtcdElector) AddOnStart(callback OnLeaderStartCallback) {
	e.onStart = append(e.onStart, callback)
}

// AddOnStop регистрирует callback, который вызывается при потере лидерства
func (e *EtcdElector) AddOnStop(callback OnLeaderStopCallback) {
	e.onStop = append(e.onStop, callback)
}
