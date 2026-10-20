package input

import (
	"testing"

	"github.com/go-playground/validator/v10"
)

func TestInputSchemaValidation(t *testing.T) {
	tests := []struct {
		name    string
		payload InputSchema
		wantErr bool
	}{
		{
			name:    "queries only",
			payload: InputSchema{Queries: []Query{{Text: "cats"}}, MaxResultsPerQuery: 8},
		},
		{
			name:    "video ids only",
			payload: InputSchema{VideoIDs: []string{"abc123"}, MaxResultsPerQuery: 50},
		},
		{
			name:    "neither queries nor video ids",
			payload: InputSchema{MaxResultsPerQuery: 8},
			wantErr: true,
		},
		{
			name:    "empty video id",
			payload: InputSchema{VideoIDs: []string{""}, MaxResultsPerQuery: 8},
			wantErr: true,
		},
	}
	v := validator.New()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := v.Struct(tt.payload)
			if (err != nil) != tt.wantErr {
				t.Fatalf("wantErr=%v, got %v", tt.wantErr, err)
			}
		})
	}
}
