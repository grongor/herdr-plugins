package main

import (
	"testing"

	"github.com/grongor/herdr-plugins/shared/manifesttest"
)

func TestManifest(t *testing.T) {
	manifesttest.Check(t, manifesttest.Dir(t), version)
}
