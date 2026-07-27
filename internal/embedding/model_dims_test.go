package embedding

import "testing"

func TestExpectedDenseDims(t *testing.T) {
	cases := []struct {
		model string
		dims  int
		known bool
	}{
		{ModelMiniLM, DefaultDims, true},
		{ModelBGEM3, BGEM3Dims, true},
		{"not-a-model", 0, false},
		{"", 0, false},
	}
	for _, c := range cases {
		d, ok := ExpectedDenseDims(c.model)
		if d != c.dims || ok != c.known {
			t.Errorf("ExpectedDenseDims(%q) = (%d, %v); want (%d, %v)", c.model, d, ok, c.dims, c.known)
		}
	}
}

func TestModelForDenseDims(t *testing.T) {
	cases := []struct {
		dims  int
		model string
		known bool
	}{
		{DefaultDims, ModelMiniLM, true},
		{BGEM3Dims, ModelBGEM3, true},
		{7, "", false},
		{0, "", false},
	}
	for _, c := range cases {
		m, ok := ModelForDenseDims(c.dims)
		if m != c.model || ok != c.known {
			t.Errorf("ModelForDenseDims(%d) = (%q, %v); want (%q, %v)", c.dims, m, ok, c.model, c.known)
		}
	}
}
