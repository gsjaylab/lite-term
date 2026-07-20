package terminal

// ValidUsername accepts the portable subset used by fnOS accounts and prevents
// a username from ever being interpreted as SSH syntax.
func ValidUsername(value string) bool {
	if len(value) == 0 || len(value) > 32 {
		return false
	}
	if !isASCIIAlpha(value[0]) && value[0] != '_' {
		return false
	}
	for index := 1; index < len(value); index++ {
		char := value[index]
		if !isASCIIAlpha(char) && (char < '0' || char > '9') && char != '_' && char != '-' {
			return false
		}
	}
	return true
}

func isASCIIAlpha(char byte) bool {
	return char >= 'a' && char <= 'z' || char >= 'A' && char <= 'Z'
}
