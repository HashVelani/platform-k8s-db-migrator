package decision

import (
	"strings"
	"testing"

	"github.com/your-org/platform-k8s-db-migrator/init-container/internal/region"
)

func TestDecide_NotImplemented(t *testing.T) {
	result, err := Decide(region.Primary, "", "/some/path")
	if err == nil {
		t.Fatal("expected error from Phase 2 stub, got nil")
	}
	if !strings.Contains(err.Error(), "Phase 2") {
		t.Errorf("expected error to mention Phase 2, got: %v", err)
	}
	if result != "" {
		t.Errorf("expected empty result from stub, got: %q", result)
	}
}
