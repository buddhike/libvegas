package consumer

import "context"

type runnable interface {
	Start(context.Context) error
	WaitForShutdownCompletion()
}
