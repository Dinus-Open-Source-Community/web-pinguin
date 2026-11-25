package main

import (
	"errors"
	"testing"
)

func TestImportErrorClass(t *testing.T) {
	for _, class := range []string{"permanent_input_error", "transient_database_error", "configuration_error"} {
		t.Run(class, func(t *testing.T) {
			err := classifyImportError(class, errors.New("fixture"))
			if got := importErrorClass(err); got != class {
				t.Fatalf("importErrorClass() = %q, want %q", got, class)
			}
		})
	}
	if got := importErrorClass(errors.New("filesystem failure")); got != "startup_or_filesystem_error" {
		t.Fatalf("unclassified importErrorClass() = %q", got)
	}
}
