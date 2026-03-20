package agentrun_test

import (
	"testing"

	"github.com/leomorpho/docket/internal/agentrun"
	"github.com/leomorpho/docket/internal/agentrun/codex"
	"github.com/leomorpho/docket/internal/agentrun/monitor"
	"github.com/leomorpho/docket/internal/agentrun/orchestrate"
	"github.com/leomorpho/docket/internal/agentrun/selector"
	"github.com/leomorpho/docket/internal/agentrun/validate"
)

func TestArchitectureExposesSmallComposableInterfaces(t *testing.T) {
	t.Parallel()

	var _ agentrun.Adapter = (*codex.Runner)(nil)
	var _ agentrun.Monitor = (*monitor.Observer)(nil)
	var _ agentrun.Selector = (*selector.Service)(nil)
	var _ agentrun.Validator = (*validate.Service)(nil)
	var _ agentrun.Orchestrator = (*orchestrate.Service)(nil)
}
