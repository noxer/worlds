package world

import "testing"

func TestParseLight(t *testing.T) {
	l, ok, err := parseLight(Props{"light": 2.5, "light_color": "#ff80ff00"})
	if err != nil || !ok {
		t.Fatalf("parseLight: %v %v", ok, err)
	}
	if l.Radius != 2.5 || l.R != 128.0/255 || l.G != 1 || l.B != 0 {
		t.Errorf("got %+v", l)
	}
	if l, ok, _ := parseLight(Props{"light": 1.0}); !ok || l.R != defaultLightColor[0] {
		t.Errorf("default colour: %+v", l)
	}
	if _, ok, err := parseLight(Props{"solid": true}); ok || err != nil {
		t.Error("no light property should be no light")
	}
	for _, bad := range []Props{{"light": -1.0}, {"light": "big"}, {"light": 1.0, "light_color": "#12"}} {
		if _, _, err := parseLight(bad); err == nil {
			t.Errorf("%v: want an error", bad)
		}
	}
}

func TestParseLighting(t *testing.T) {
	for s, want := range map[string]Lighting{"": Indoor, "indoor": Indoor, "outdoor": Outdoor, "dark": Dark} {
		if got, err := ParseLighting(s); err != nil || got != want {
			t.Errorf("%q: %v %v", s, got, err)
		}
	}
	if _, err := ParseLighting("dim"); err == nil {
		t.Error("want an error")
	}
}
