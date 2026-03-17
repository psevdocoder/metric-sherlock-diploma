package leaderelection

// LocalElector представляет собой реализацию интерфейса LeaderElector для локальной разработки
type LocalElector struct {
	onStart []OnLeaderStartCallback
	onStop  []OnLeaderStopCallback
}

// NewLocalElector конструктор LocalElector
func NewLocalElector() *LocalElector {
	return &LocalElector{}
}

// AddOnStart регистрирует callback, который вызывается при получении лидерства
func (e *LocalElector) AddOnStart(callback OnLeaderStartCallback) {
	e.onStart = append(e.onStart, callback)
}

// AddOnStop регистрирует callback, который вызывается при потере лидерства
func (e *LocalElector) AddOnStop(callback OnLeaderStopCallback) {
	e.onStop = append(e.onStop, callback)
}

// Run запускает callbacks зарегистрированные для локального elector
func (e *LocalElector) Run() error {
	for _, onStart := range e.onStart {
		go onStart()
	}
	return nil
}
