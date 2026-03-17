package leaderelection

type OnLeaderStartCallback func()

type OnLeaderStopCallback func()

type LeaderElector interface {
	AddOnStart(callback OnLeaderStartCallback)
	AddOnStop(callback OnLeaderStopCallback)
	Run() error
}
