package terminal

import (
	"strings"
	"testing"
)

func TestValidUsername(t *testing.T) {
	for _, value := range []string{"admin", "Admin-2", "_service", "user_01"} {
		if !ValidUsername(value) {
			t.Errorf("rejected %q", value)
		}
	}
	for _, value := range []string{"", "-admin", "a.b", "用户", strings.Repeat("a", 33)} {
		if ValidUsername(value) {
			t.Errorf("accepted %q", value)
		}
	}
}
