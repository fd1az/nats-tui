package messages

import "context"

type Watcher interface {
	Watch(context.Context, []string) (<-chan Record, error)
}

type WatchHandler struct {
	watcher Watcher
}

func NewWatchHandler(watcher Watcher) WatchHandler {
	if watcher == nil {
		panic("nil message watcher")
	}
	return WatchHandler{watcher: watcher}
}

func (h WatchHandler) Handle(ctx context.Context, subjects []string) (<-chan Record, error) {
	return h.watcher.Watch(ctx, subjects)
}
