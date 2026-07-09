package bridge

import (
	"context"

	"heddle/pkg/runtime"
)

// GoBridge isola a execução física de chamadas de funções Go nativas a partir da VM.
type GoBridge interface {
	Call(ctx context.Context, fnName string, input *runtime.Frame, namedArgs map[string]any, positionalArgs []any) (*runtime.Frame, error)
}
