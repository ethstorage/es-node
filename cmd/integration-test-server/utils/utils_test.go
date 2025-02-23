package utils

import (
	"testing"
)

func Test_CheckKnownFailure(t *testing.T) {
	tests := []struct {
		name        string
		filePath    string
		expectCount int
	}{
		{
			name:        "success",
			filePath:    "./testdata/success.log",
			expectCount: 0,
		},
		{
			name:        "diffnotmatch",
			filePath:    "./testdata/diffnotmatch.log",
			expectCount: 1,
		},
		{
			name:        "invalidsamples0",
			filePath:    "./testdata/invalidsamples0.log",
			expectCount: 1,
		},
		{
			name:        "invalidsamples1",
			filePath:    "./testdata/invalidsamples1.log",
			expectCount: 1,
		},
		{
			name:        "invalidsamples2",
			filePath:    "./testdata/invalidsamples2.log",
			expectCount: 1,
		},
		{
			name:        "minedtstoosmall",
			filePath:    "./testdata/minedtstoosmall.log",
			expectCount: 1,
		},
	}
	for _, test := range tests {
		count, err := CheckKnowFailure(test.filePath)
		if err != nil {
			t.Errorf("%s test fail, error: %s", test.name, err.Error())
		}
		if count != test.expectCount {
			t.Errorf("%s test fail, expect count is %d and real count is %d", test.name, test.expectCount, count)
		}
	}
}
