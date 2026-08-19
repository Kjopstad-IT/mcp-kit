package mcpkit

import (
	"context"
	"fmt"
	"math"
)

// Progress is a surface-neutral update from a long-running handler. Total is
// zero when the amount of work is unknown.
type Progress struct {
	Message string
	Current float64
	Total   float64
}

type progressReporter func(context.Context, Progress) error

type progressKey struct{}

// ReportProgress sends an update to the active surface. It is a no-op when the
// caller did not request progress or the handler runs without a surface.
func ReportProgress(ctx context.Context, progress Progress) error {
	if ctx == nil {
		return fmt.Errorf("mcp-kit: nil progress context")
	}
	if math.IsNaN(progress.Current) || math.IsInf(progress.Current, 0) || progress.Current < 0 {
		return fmt.Errorf("mcp-kit: invalid current progress %v", progress.Current)
	}
	if math.IsNaN(progress.Total) || math.IsInf(progress.Total, 0) || progress.Total < 0 {
		return fmt.Errorf("mcp-kit: invalid total progress %v", progress.Total)
	}
	reporter, _ := ctx.Value(progressKey{}).(progressReporter)
	if reporter == nil {
		return nil
	}
	return reporter(ctx, progress)
}

func withProgressReporter(ctx context.Context, reporter progressReporter) context.Context {
	return context.WithValue(ctx, progressKey{}, reporter)
}
