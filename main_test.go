package main

import (
	"testing"

	"go.uber.org/fx"
)

func TestApplicationGraph(t *testing.T) {
	// ValidateApp resolves types without running constructors or opening services.
	if err := fx.ValidateApp(appOptions...); err != nil {
		t.Fatal(err)
	}
}
