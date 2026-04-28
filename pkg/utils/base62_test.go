package utils

import (
	"testing"
)

func TestEncode(t *testing.T) {
	tests := []struct {
		name     string
		input    uint64
		expected string
	}{
		// 边界情况
		{"zero", 0, "0"},
		{"one", 1, "1"},
		{"max_digit_9", 9, "9"},
		// 索引边界
		{"max_digit_z", 9, "9"},  // 9 -> '9'
		{"max_digit_a", 10, "a"}, // 10 -> 'a'
		{"max_digit_A", 36, "A"}, // 36 -> 'A'
		{"last_char_Z", 61, "Z"}, // 61 -> 'Z'
		// 常规数字
		{"sixty_two", 62, "10"},   // 62 = 1*62 + 0 -> "10"
		{"sixty_three", 63, "11"}, // 63 = 1*62 + 1 -> "11"
		{"example_12345", 12345, "3d7"},
		// 较大数字
		{"large_number", 9876543210, "aMoY42"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Encode(tt.input)
			if result != tt.expected {
				t.Errorf("Encode(%d) = %q, want %q", tt.input, result,
					tt.expected)
			}
		})
	}
}

func TestDecode(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		expected    uint64
		expectError bool
	}{
		// 边界情况
		{"zero", "0", 0, false},
		// 常规字符串
		{"simple_10", "10", 62, false},
		{"example_3d7", "3d7", 12345, false},
		{"large", "aMoY42", 9876543210, false},
		// 错误情况
		{"invalid_char", "abc$def", 0, true},
		{"empty", "", 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := Decode(tt.input)
			if tt.expectError {
				if err == nil {
					t.Errorf("Decode(%q) expected error, got nil", tt.input)
				}
			} else {
				if err != nil {
					t.Errorf("Decode(%q) unexpected error: %v", tt.input, err)
				}
				if result != tt.expected {
					t.Errorf("Decode(%q) = %d, want %d", tt.input, result,
						tt.expected)
				}
			}
		})
	}
}

// TestEncodeDecodeRoundTrip 测试编码后再解码应返回原值
func TestEncodeDecodeRoundTrip(t *testing.T) {
	numbers := []uint64{0, 1, 10, 61, 62, 63, 100, 1000, 12345, 999999,
		18446744073709551615}

	for _, num := range numbers {
		encoded := Encode(num)
		decoded, err := Decode(encoded)
		if err != nil {
			t.Errorf("Round trip failed for %d: encode=%q, decode error: %v",
				num, encoded, err)
		}
		if decoded != num {
			t.Errorf("Round trip failed for %d: got %d after encode(%q)", num,
				decoded, encoded)
		}
	}
}
