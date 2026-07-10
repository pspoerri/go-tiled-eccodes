package grib_test

import (
	"testing"

	grib "github.com/pspoerri/go-tiled-eccodes"
	"github.com/pspoerri/go-tiled-eccodes/writer"
)

type parameterResolverFunc func(grib.ParameterKey) (grib.Parameter, bool)

func (f parameterResolverFunc) ResolveParameter(key grib.ParameterKey) (grib.Parameter, bool) {
	return f(key)
}

func TestParameterResolver(t *testing.T) {
	grid := writer.NewLatLon(2, 2, 1, 0, 1, 1)
	field := minimalField(grid, []float64{1, 2, 3, 4})
	field.ParameterCategory = 1
	field.ParameterNumber = 2
	data, err := writer.Single(field)
	if err != nil {
		t.Fatalf("Single: %v", err)
	}
	file, err := grib.FromBytes(data)
	if err != nil {
		t.Fatalf("FromBytes: %v", err)
	}
	defer file.Close()
	message := file.Messages()[0]

	key := message.ParameterKey()
	if key.Centre != 78 || key.Discipline != 0 || key.Category != 1 || key.Number != 2 {
		t.Fatalf("ParameterKey = %+v", key)
	}
	want := grib.Parameter{ID: 123, Name: "test parameter", ShortName: "tp", Units: "1"}
	got, ok := message.ResolveParameter(parameterResolverFunc(func(resolved grib.ParameterKey) (grib.Parameter, bool) {
		if resolved != key {
			t.Fatalf("resolver key = %+v, want %+v", resolved, key)
		}
		return want, true
	}))
	if !ok || got != want {
		t.Fatalf("ResolveParameter = %+v, %v", got, ok)
	}
	if _, ok := message.ResolveParameter(nil); ok {
		t.Fatal("nil resolver succeeded")
	}
}
