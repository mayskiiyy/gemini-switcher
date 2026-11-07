package notify

import (
	"testing"
)

func TestSendNotification(t *testing.T) {
	err := SendNotification("Gemini Switcher", "Test notification")
	if err != nil {
		t.Logf("Notification warning: %v", err)
	}
}
