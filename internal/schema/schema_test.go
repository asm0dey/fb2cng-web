package schema

import "testing"

func TestIsDefaultBool(t *testing.T) {
	o := Option{Kind: KindBool, Default: false}
	if !o.IsDefault(false) {
		t.Fatal("false should equal default false")
	}
	if o.IsDefault(true) {
		t.Fatal("true should not equal default false")
	}
}

func TestIsDefaultIntAcrossFloat64(t *testing.T) {
	// JSON numbers decode to float64; the editor supplies an int.
	o := Option{Kind: KindInt, Default: float64(75)}
	if !o.IsDefault(75) {
		t.Fatal("int 75 should equal default float64(75)")
	}
	if o.IsDefault(80) {
		t.Fatal("80 should not equal default 75")
	}
}

func TestIsDefaultString(t *testing.T) {
	o := Option{Kind: KindString, Default: "normal"}
	if !o.IsDefault("normal") {
		t.Fatal("string should equal identical default")
	}
	if o.IsDefault("flat") {
		t.Fatal("different string should not equal default")
	}
}
