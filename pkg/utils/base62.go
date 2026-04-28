package utils

import "errors"

var ErrInvalidBase62 = errors.New("base62: invalid character in string")

const charset = "0123456789abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"

var base62chars = make(map[byte]int)

func init() {
	for i := 0; i < len(charset); i++ {
		base62chars[charset[i]] = i
	}
}

func Encode(num uint64) string {
	if num == 0 {
		return "0"
	}

	var digits []byte

	for num > 0 {
		remainder := num % 62
		digits = append(digits, charset[remainder])
		num /= 62
	}

	reverse(digits)

	return string(digits)
}

func Decode(s string) (uint64, error) {
	if len(s) == 0 {
		return 0, errors.New("empty string")
	}

	result := uint64(0)

	for i := 0; i < len(s); i++ {
		char := s[i]
		digit, ok := base62chars[char]
		if !ok {
			return 0, ErrInvalidBase62
		}
		result = result*62 + uint64(digit)
	}

	return result, nil
}

func reverse(s []byte) {
	left, right := 0, len(s)-1
	for left < right {
		s[left], s[right] = s[right], s[left]
		left++
		right--
	}
}
