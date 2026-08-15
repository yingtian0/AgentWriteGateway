package postgres

import "testing"

func TestUniqueViolationClassificationIsFailClosed(t *testing.T) {
	if isUniqueViolation(nil) {
		t.Fatal("nil error classified as unique violation")
	}
}
