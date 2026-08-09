package alerts

import (
	"context"
	"errors"
)

type MultiNotifier []Notifier

func (notifiers MultiNotifier) Notify(ctx context.Context, record Record) error {
	var failures []error
	for _, notifier := range notifiers {
		if notifier != nil {
			failures = append(failures, notifier.Notify(ctx, record))
		}
	}
	return errors.Join(failures...)
}
