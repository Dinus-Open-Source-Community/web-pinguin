package httpapi

import "testing"

func TestValidStatsRepositoryID(t *testing.T) {
	for _, tc := range []struct {
		value string
		want  bool
	}{
		{value: "", want: true},
		{value: "00000000-0000-4000-8000-000000000001", want: true},
		{value: "not-a-uuid", want: false},
		{value: "00000000-0000-4000-8000-00000000000g", want: false},
		{value: "00000000000040008000000000000001", want: false},
	} {
		t.Run(tc.value, func(t *testing.T) {
			if got := validStatsRepositoryID(tc.value); got != tc.want {
				t.Fatalf("validStatsRepositoryID(%q) = %v, want %v", tc.value, got, tc.want)
			}
		})
	}
}
