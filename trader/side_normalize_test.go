package trader

import "testing"

func TestNormalizeInternalSide(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"long", "long"},
		{"short", "short"},
		{"LONG", "long"},
		{"SHORT", "short"},
		{"Long", "long"},
		{"Short", "short"},
		{"buy", "long"},
		{"sell", "short"},
		{"BUY", "long"},
		{"SELL", "short"},
		{"l", "long"},
		{"s", "short"},
		{"", ""},
		{"invalid", "invalid"},
	}

	for _, test := range tests {
		result := NormalizeInternalSide(test.input)
		if result != test.expected {
			t.Errorf("NormalizeInternalSide(%q) = %q, want %q", test.input, result, test.expected)
		}
	}
}

func TestNormalizePositionSide(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"long", "LONG"},
		{"short", "SHORT"},
		{"LONG", "LONG"},
		{"SHORT", "SHORT"},
		{"Long", "LONG"},
		{"Short", "SHORT"},
		{"buy", "LONG"},
		{"sell", "SHORT"},
		{"BUY", "LONG"},
		{"SELL", "SHORT"},
		{"", ""},
		{"invalid", "INVALID"},
	}

	for _, test := range tests {
		result := NormalizePositionSide(test.input)
		if result != test.expected {
			t.Errorf("NormalizePositionSide(%q) = %q, want %q", test.input, result, test.expected)
		}
	}
}